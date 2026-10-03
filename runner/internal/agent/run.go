package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"

	"leetforce/judge/engine"
	"leetforce/judge/verdict"
	"leetforce/queue"
	"leetforce/runner/internal/problems"
)

// verdictOK is reported for a custom-input run that ended cleanly: there is no
// expected output, so it is not an AC.
const verdictOK = "OK"

// processRun handles a Run job. It differs from a submission in three ways: it
// only ever uses the problem's sample tests or the user's own input, its result
// goes to the run's Redis key (never the results stream, so it is never stored
// as a verdict), and it may include input and output details.
func (a *Agent) processRun(ctx context.Context, log *slog.Logger, d *queue.Delivery) {
	id := d.Job.SubmissionID
	judgeCtx, cancelJudge := context.WithCancel(ctx)
	defer cancelJudge()
	hbCtx, stopHeartbeat := context.WithCancel(ctx)
	var lost atomic.Bool
	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		a.heartbeat(hbCtx, log, d.ID, &lost, cancelJudge)
	}()

	if err := a.q.SetRun(ctx, id, queue.RunState{Status: queue.RunJudging}); err != nil {
		log.Warn("set run judging failed; continuing", "err", err)
	}
	log.Info("running", "problem", d.Job.Problem, "language", d.Job.Language, "custom", d.Job.Custom)
	res, permanent, err := a.run(judgeCtx, d.Job)
	stopHeartbeat()
	<-hbDone

	if lost.Load() {
		log.Warn("run was taken over by another runner; discarding result")
		return
	}
	switch {
	case err == nil:
	case permanent || d.Deliveries >= a.cfg.MaxAttempts:
		log.Error("run failed; reporting IE", "err", err)
		res = queue.RunResult{Verdict: VerdictInternalError}
	default:
		log.Error("host failure; leaving run pending for redelivery", "err", err)
		return
	}
	if err := a.q.SetRun(ctx, id, queue.RunState{Status: queue.RunDone, Result: &res}); err != nil {
		log.Error("store run result failed; leaving run pending", "err", err)
		return
	}
	log.Info("run finished", "verdict", res.Verdict)
	a.ack(ctx, log, d.ID)
}

// run loads the problem and executes the job. permanent is true when no retry
// could succeed.
func (a *Agent) run(ctx context.Context, j queue.Job) (res queue.RunResult, permanent bool, err error) {
	p, err := a.cfg.Problems.Load(ctx, j.Problem, j.TestSetVersion)
	if err != nil {
		return res, errors.Is(err, problems.ErrPermanent), fmt.Errorf("load problem %q: %w", j.Problem, err)
	}
	if j.Custom {
		rep, err := a.judger.RunCustom(ctx, p, j.Language, []byte(j.Source), []byte(j.Input))
		if err != nil {
			return res, isPermanentRun(err), fmt.Errorf("run %s: %w", j.SubmissionID, err)
		}
		v := string(rep.Verdict)
		if rep.Verdict == verdict.Completed {
			v = verdictOK
		}
		res = queue.RunResult{
			Verdict: v, RuntimeMS: rep.Time.Milliseconds(), MemoryKB: rep.Memory / 1024,
			Stdout: rep.Stdout, Stderr: rep.Stderr,
		}
		if rep.Verdict == verdict.CE {
			res.CompileOutput = rep.CompileOutput
		}
		return res, false, nil
	}

	samples := *p
	samples.Tests = nil
	for _, t := range p.Tests {
		if t.Sample {
			samples.Tests = append(samples.Tests, t)
		}
	}
	if len(samples.Tests) == 0 {
		return res, true, errors.New("problem has no sample tests")
	}
	rep, err := a.judger.Judge(ctx, &samples, j.Language, []byte(j.Source), engine.Options{ContinueOnFail: true, Detail: true})
	if err != nil {
		return res, isPermanentRun(err), fmt.Errorf("run %s: %w", j.SubmissionID, err)
	}
	res = queue.RunResult{
		Verdict:   string(rep.Overall.Verdict),
		RuntimeMS: rep.Overall.Time.Milliseconds(),
		MemoryKB:  rep.Overall.Memory / 1024,
	}
	if rep.Overall.Verdict == verdict.CE {
		res.CompileOutput = rep.CompileOutput
	}
	for _, c := range rep.Cases {
		rc := queue.RunCase{Name: c.Name, Verdict: string(c.Verdict)}
		if c.Detail != nil {
			rc.Input, rc.Expected, rc.Actual, rc.Stderr = c.Detail.Input, c.Detail.Expected, c.Detail.Actual, c.Detail.Stderr
		}
		res.Cases = append(res.Cases, rc)
	}
	return res, false, nil
}

func isPermanentRun(err error) bool {
	return errors.Is(err, engine.ErrUnknownLanguage) || errors.Is(err, engine.ErrSourceTooLarge) || errors.Is(err, engine.ErrInputTooLarge)
}
