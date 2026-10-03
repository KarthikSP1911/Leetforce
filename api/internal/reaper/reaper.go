// Package reaper re-queues submissions that were stored but never queued.
//
// POST /submissions inserts the row and then queues the job. If the API dies
// between the two, the row is "queued" forever with no job behind it. Such rows
// are exactly the ones whose enqueued_at is still NULL after a grace period.
// The reaper sweeps once at startup (a crash is noticed when the API comes
// back) and then on a slow interval; a fast poll would keep Neon from
// suspending when idle.
package reaper

import (
	"context"
	"log/slog"
	"time"

	"leetforce/api/internal/metrics"
	"leetforce/api/internal/store"
	"leetforce/queue"
)

// Store is the database side.
type Store interface {
	ReapUnqueued(ctx context.Context, grace time.Duration, limit int, enqueue func(context.Context, store.Unqueued) error) (int, error)
}

// Enqueuer puts a job on the queue.
type Enqueuer interface {
	Enqueue(ctx context.Context, j queue.Job) (string, error)
}

// Config tunes the reaper. Zero values take the defaults noted per field.
type Config struct {
	Every time.Duration // sweep interval after the first sweep (default 15m)
	Grace time.Duration // a row younger than this is left alone: its request may still be running (default 2m)
	Batch int           // rows per sweep (default 50)
}

// Reaper runs the sweeps.
type Reaper struct {
	st  Store
	q   Enqueuer
	log *slog.Logger
	cfg Config
}

// New builds a Reaper.
func New(st Store, q Enqueuer, log *slog.Logger, cfg Config) *Reaper {
	if cfg.Every <= 0 {
		cfg.Every = 15 * time.Minute
	}
	if cfg.Grace <= 0 {
		cfg.Grace = 2 * time.Minute
	}
	if cfg.Batch <= 0 {
		cfg.Batch = 50
	}
	return &Reaper{st: st, q: q, log: log, cfg: cfg}
}

// Run sweeps immediately, then every cfg.Every until ctx is cancelled.
func (r *Reaper) Run(ctx context.Context) {
	t := time.NewTicker(r.cfg.Every)
	defer t.Stop()
	for {
		r.Sweep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Sweep re-queues one batch of orphaned submissions and returns how many.
func (r *Reaper) Sweep(ctx context.Context) int {
	n, err := r.st.ReapUnqueued(ctx, r.cfg.Grace, r.cfg.Batch, func(ctx context.Context, u store.Unqueued) error {
		_, err := r.q.Enqueue(ctx, queue.Job{
			SubmissionID: u.ID, Problem: u.Problem, Language: u.Language,
			Source: u.Source, TestSetVersion: u.TestSetVersion,
		})
		return err
	})
	if err != nil && ctx.Err() == nil {
		r.log.Error("reaper sweep", "requeued", n, "err", err)
	}
	if n > 0 {
		metrics.ReaperRequeued.Add(float64(n))
		r.log.Warn("re-queued submissions that were stored but never queued", "count", n)
	}
	return n
}
