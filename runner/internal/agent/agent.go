// Package agent is the runner's main loop: pull a job from the queue, judge it,
// report the verdict, acknowledge the job. It talks only to the queue (Redis);
// it never connects to a database. Problems are read from a local directory.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"time"

	"leetforce/judge/engine"
	"leetforce/judge/problem"
	"leetforce/judge/verdict"
	"leetforce/queue"
)

// VerdictInternalError is reported when a job can never be judged (unknown
// problem or language, oversized source) or the host kept failing on it. It is
// not one of the judge's verdicts: it says the platform, not the program, failed.
const VerdictInternalError = "IE"

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Judger judges one submission. *engine.Engine satisfies it; tests use fakes.
type Judger interface {
	Judge(ctx context.Context, p *problem.Problem, language string, source []byte, opts engine.Options) (*engine.Report, error)
}

// JobQueue is the part of *queue.Queue the agent uses.
type JobQueue interface {
	Receive(ctx context.Context, consumer string, block time.Duration) (*queue.Delivery, error)
	Ack(ctx context.Context, id string) error
	Touch(ctx context.Context, consumer, id string) error
	Publish(ctx context.Context, r queue.Result) (bool, error)
	Published(ctx context.Context, submissionID string) (bool, error)
}

// Config configures an Agent.
type Config struct {
	ID             string        // unique consumer name
	ProblemsDir    string        // directory holding problems/<slug>
	PollBlock      time.Duration // how long Receive waits for a job (default 5s)
	HeartbeatEvery time.Duration // claim refresh interval; must be well under the queue's MinIdle (default 10s)
	MaxAttempts    int64         // deliveries before an IE verdict is reported (default 3)
	Logger         *slog.Logger
}

// Agent pulls and judges jobs.
type Agent struct {
	cfg    Config
	q      JobQueue
	judger Judger
	log    *slog.Logger
}

// New returns an Agent.
func New(q JobQueue, j Judger, cfg Config) *Agent {
	if cfg.PollBlock <= 0 {
		cfg.PollBlock = 5 * time.Second
	}
	if cfg.HeartbeatEvery <= 0 {
		cfg.HeartbeatEvery = 10 * time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Agent{cfg: cfg, q: q, judger: j, log: cfg.Logger.With("runner", cfg.ID)}
}

// Run processes jobs until ctx is cancelled. A job already started is finished
// (it runs on a context that ignores the cancellation) so shutdown does not
// abandon work; a hard kill is covered by the queue reclaiming the job.
func (a *Agent) Run(ctx context.Context) error {
	a.log.Info("runner started")
	for ctx.Err() == nil {
		d, err := a.q.Receive(ctx, a.cfg.ID, a.cfg.PollBlock)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			a.log.Error("receive failed", "err", err)
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
			continue
		}
		if d == nil {
			continue
		}
		a.Process(context.WithoutCancel(ctx), d)
	}
	a.log.Info("runner stopped")
	return nil
}

// Process judges one delivered job and reports the outcome. Failures that a
// retry could fix leave the job unacknowledged so the queue redelivers it.
func (a *Agent) Process(ctx context.Context, d *queue.Delivery) {
	log := a.log.With("submission", d.Job.SubmissionID, "entry", d.ID, "delivery", d.Deliveries, "reclaimed", d.Reclaimed)

	if done, err := a.q.Published(ctx, d.Job.SubmissionID); err != nil {
		log.Error("check existing verdict failed; leaving job pending", "err", err)
		return
	} else if done {
		log.Info("verdict already recorded; acknowledging without judging")
		a.ack(ctx, log, d.ID)
		return
	}

	judgeCtx, cancelJudge := context.WithCancel(ctx)
	defer cancelJudge()
	hbCtx, stopHeartbeat := context.WithCancel(ctx)
	var lost atomic.Bool
	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		a.heartbeat(hbCtx, log, d.ID, &lost, cancelJudge)
	}()

	log.Info("judging", "problem", d.Job.Problem, "language", d.Job.Language)
	res, permanent, err := a.judge(judgeCtx, d.Job)
	stopHeartbeat()
	<-hbDone

	if lost.Load() {
		log.Warn("job was taken over by another runner; discarding result")
		return
	}
	switch {
	case err == nil:
	case permanent:
		log.Error("job cannot be judged", "err", err)
		res = a.internalError(d.Job, err)
	case d.Deliveries >= a.cfg.MaxAttempts:
		log.Error("host failed on the last attempt; reporting IE", "err", err)
		res = a.internalError(d.Job, err)
	default:
		log.Error("host failure; leaving job pending for redelivery", "err", err)
		return
	}

	first, err := a.q.Publish(ctx, res)
	if err != nil {
		log.Error("publish failed; leaving job pending", "err", err)
		return
	}
	log.Info("verdict reported", "verdict", res.Verdict, "recorded", first, "runtime_ms", res.RuntimeMS, "memory_kb", res.MemoryKB)
	a.ack(ctx, log, d.ID)
}

func (a *Agent) ack(ctx context.Context, log *slog.Logger, id string) {
	if err := a.q.Ack(ctx, id); err != nil {
		// The verdict is recorded, so a redelivery acknowledges without judging.
		log.Error("ack failed", "err", err)
	}
}

// heartbeat keeps the claim alive. If the job was taken away it marks the job
// lost and cancels the judging.
func (a *Agent) heartbeat(ctx context.Context, log *slog.Logger, id string, lost *atomic.Bool, cancelJudge context.CancelFunc) {
	t := time.NewTicker(a.cfg.HeartbeatEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			err := a.q.Touch(ctx, a.cfg.ID, id)
			switch {
			case err == nil:
			case errors.Is(err, queue.ErrLost):
				lost.Store(true)
				cancelJudge()
				return
			case ctx.Err() != nil:
				return
			default:
				log.Warn("heartbeat failed; will retry", "err", err)
			}
		}
	}
}

// judge loads the problem and runs the engine. permanent is true when no retry
// could succeed (the job itself is bad).
func (a *Agent) judge(ctx context.Context, j queue.Job) (res queue.Result, permanent bool, err error) {
	if !slugRe.MatchString(j.Problem) {
		return res, true, fmt.Errorf("invalid problem slug %q", j.Problem)
	}
	p, err := problem.Load(filepath.Join(a.cfg.ProblemsDir, j.Problem))
	if err != nil {
		return res, true, fmt.Errorf("load problem %q: %w", j.Problem, err)
	}
	rep, err := a.judger.Judge(ctx, p, j.Language, []byte(j.Source), engine.Options{})
	if err != nil {
		perm := errors.Is(err, engine.ErrUnknownLanguage) || errors.Is(err, engine.ErrSourceTooLarge)
		return res, perm, fmt.Errorf("judge %s: %w", j.SubmissionID, err)
	}
	passed := 0
	for _, c := range rep.Cases {
		if c.Verdict == verdict.AC {
			passed++
		}
	}
	res = queue.Result{
		SubmissionID:   j.SubmissionID,
		Verdict:        string(rep.Overall.Verdict),
		RuntimeMS:      rep.Overall.Time.Milliseconds(),
		MemoryKB:       rep.Overall.Memory / 1024,
		TestSetVersion: rep.TestSetVersion,
		Passed:         passed,
		Total:          len(p.Tests),
		RunnerID:       a.cfg.ID,
	}
	if rep.Overall.Verdict == verdict.CE {
		res.CompileOutput = rep.CompileOutput
	}
	return res, false, nil
}

// internalError builds the IE result. It carries no error text: the cause is in
// the runner's log, not in what a user can read.
func (a *Agent) internalError(j queue.Job, _ error) queue.Result {
	return queue.Result{SubmissionID: j.SubmissionID, Verdict: VerdictInternalError, RunnerID: a.cfg.ID}
}
