// Package ingest moves verdicts from the Redis results stream into Postgres,
// and turns dead-lettered jobs into internal-error (IE) verdicts, so every
// submission ends with exactly one verdict.
package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"leetforce/api/internal/store"
	"leetforce/queue"
)

// Source is the queue side: the results and dead-letter streams.
type Source interface {
	ReceiveResult(ctx context.Context, consumer string, block time.Duration) (*queue.ResultDelivery, error)
	AckResult(ctx context.Context, id string) error
	ReceiveDead(ctx context.Context, consumer string, block time.Duration) (*queue.DeadDelivery, error)
	AckDead(ctx context.Context, id string) error
}

// Recorder is the database side.
type Recorder interface {
	RecordVerdict(ctx context.Context, v store.VerdictRecord) (bool, error)
}

// Config tunes the loop. Zero values take the defaults noted per field.
type Config struct {
	Consumer  string        // consumer name in the API group (required)
	Block     time.Duration // how long to wait for a verdict per poll (default 5s)
	DeadEvery time.Duration // how often to look at the dead-letter stream (default 30s)
	Backoff   time.Duration // pause after a failed poll (default 2s)
}

// DeadLetterRunnerID marks verdicts the API wrote for a job no runner finished.
const DeadLetterRunnerID = "dead-letter"

// Ingester runs the loop.
type Ingester struct {
	src Source
	rec Recorder
	log *slog.Logger
	cfg Config
}

// New builds an Ingester.
func New(src Source, rec Recorder, log *slog.Logger, cfg Config) *Ingester {
	if cfg.Block == 0 {
		cfg.Block = 5 * time.Second
	}
	if cfg.DeadEvery == 0 {
		cfg.DeadEvery = 30 * time.Second
	}
	if cfg.Backoff == 0 {
		cfg.Backoff = 2 * time.Second
	}
	return &Ingester{src: src, rec: rec, log: log, cfg: cfg}
}

// Run polls until ctx is cancelled. A failed poll or write is logged and
// retried; the entry stays pending in the queue, so it is redelivered rather
// than lost.
func (g *Ingester) Run(ctx context.Context) {
	var lastDead time.Time
	for ctx.Err() == nil {
		d, err := g.src.ReceiveResult(ctx, g.cfg.Consumer, g.cfg.Block)
		if err != nil {
			g.pause(ctx, "receive result", err)
			continue
		}
		if d != nil {
			g.HandleResult(ctx, d)
		}
		if time.Since(lastDead) >= g.cfg.DeadEvery {
			if err := g.drainDead(ctx); err != nil {
				g.pause(ctx, "receive dead letter", err)
				continue
			}
			lastDead = time.Now()
		}
	}
}

func (g *Ingester) pause(ctx context.Context, what string, err error) {
	if ctx.Err() != nil {
		return
	}
	g.log.Error(what, "err", err)
	select {
	case <-ctx.Done():
	case <-time.After(g.cfg.Backoff):
	}
}

func (g *Ingester) drainDead(ctx context.Context) error {
	for ctx.Err() == nil {
		d, err := g.src.ReceiveDead(ctx, g.cfg.Consumer, time.Millisecond)
		if err != nil {
			return err
		}
		if d == nil {
			return nil
		}
		g.HandleDead(ctx, d)
	}
	return nil
}

// HandleResult stores one verdict and acknowledges it. An entry that can never
// be stored (undecodable, invalid data) is logged and acknowledged so it does
// not block the stream; a transient failure leaves it pending to be redelivered.
func (g *Ingester) HandleResult(ctx context.Context, d *queue.ResultDelivery) {
	if d.Malformed {
		g.log.Error("dropping undecodable result", "entry", d.ID)
		g.ack(ctx, "result", d.ID, g.src.AckResult)
		return
	}
	r := d.Result
	if _, err := uuid.Parse(r.SubmissionID); err != nil {
		g.log.Error("dropping result with an invalid submission id", "entry", d.ID, "submission", r.SubmissionID)
		g.ack(ctx, "result", d.ID, g.src.AckResult)
		return
	}
	recorded, err := g.rec.RecordVerdict(ctx, store.VerdictRecord{
		SubmissionID: r.SubmissionID, Verdict: r.Verdict, RuntimeMS: r.RuntimeMS, MemoryKB: int64(min(r.MemoryKB, 1<<62)),
		Passed: r.Passed, Total: r.Total, TestSetVersion: r.TestSetVersion, RunnerID: r.RunnerID,
	})
	switch {
	case err != nil && store.IsPermanent(err):
		g.log.Error("dropping result the database rejects", "entry", d.ID, "submission", r.SubmissionID, "err", err)
	case err != nil:
		g.log.Error("store verdict, will retry", "entry", d.ID, "submission", r.SubmissionID, "err", err)
		return
	case recorded:
		g.log.Info("verdict stored", "submission", r.SubmissionID, "verdict", r.Verdict, "runner", r.RunnerID)
	default:
		g.log.Info("verdict already stored or submission unknown, ignored", "submission", r.SubmissionID)
	}
	g.ack(ctx, "result", d.ID, g.src.AckResult)
}

// HandleDead records an IE verdict for a job the runners gave up on.
func (g *Ingester) HandleDead(ctx context.Context, d *queue.DeadDelivery) {
	if d.Job == nil {
		g.log.Error("dead letter without a readable job", "entry", d.ID, "reason", d.Reason)
		g.ack(ctx, "dead letter", d.ID, g.src.AckDead)
		return
	}
	recorded, err := g.rec.RecordVerdict(ctx, store.VerdictRecord{
		SubmissionID: d.Job.SubmissionID, Verdict: "IE", RunnerID: DeadLetterRunnerID,
	})
	switch {
	case err != nil && store.IsPermanent(err):
		g.log.Error("dead letter the database rejects", "entry", d.ID, "submission", d.Job.SubmissionID, "err", err)
	case err != nil:
		g.log.Error("store IE verdict, will retry", "entry", d.ID, "submission", d.Job.SubmissionID, "err", err)
		return
	default:
		g.log.Warn("job dead-lettered", "submission", d.Job.SubmissionID, "reason", d.Reason, "deliveries", d.Deliveries, "ie_stored", recorded)
	}
	g.ack(ctx, "dead letter", d.ID, g.src.AckDead)
}

func (g *Ingester) ack(ctx context.Context, what, id string, fn func(context.Context, string) error) {
	if err := fn(ctx, id); err != nil {
		// The verdict is already stored; a redelivery is a harmless duplicate.
		g.log.Error("acknowledge "+what, "entry", id, "err", err)
	}
}
