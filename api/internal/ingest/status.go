package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"leetforce/api/internal/store"
	"leetforce/queue"
)

// StatusSource is the queue side of the status stream.
type StatusSource interface {
	StatusTail(ctx context.Context) (string, error)
	ReadStatus(ctx context.Context, after string, block time.Duration) ([]queue.StatusEvent, string, error)
}

// JudgingMarker is the database side: move a queued submission to judging.
type JudgingMarker interface {
	MarkJudging(ctx context.Context, id string) (bool, error)
}

// StatusConfig tunes the watcher. Zero values take the defaults noted per field.
type StatusConfig struct {
	Block   time.Duration // how long to wait for events per poll (default 5s)
	Backoff time.Duration // pause after a failed poll (default 2s)
}

// StatusWatcher copies "judging" events from the status stream into Postgres,
// so the SSE endpoint can read one source of truth. It is best effort and
// stateless: it starts at the newest entry, keeps no acknowledgements, and
// every API instance applies every event. The update only ever moves a queued
// submission forward, so repeats and late events are harmless, and a lost one
// only means a submission shows "queued" until its verdict arrives.
type StatusWatcher struct {
	src StatusSource
	mk  JudgingMarker
	log *slog.Logger
	cfg StatusConfig
}

// NewStatusWatcher builds a StatusWatcher.
func NewStatusWatcher(src StatusSource, mk JudgingMarker, log *slog.Logger, cfg StatusConfig) *StatusWatcher {
	if cfg.Block == 0 {
		cfg.Block = 5 * time.Second
	}
	if cfg.Backoff == 0 {
		cfg.Backoff = 2 * time.Second
	}
	return &StatusWatcher{src: src, mk: mk, log: log, cfg: cfg}
}

// Run reads status events until ctx is cancelled.
func (w *StatusWatcher) Run(ctx context.Context) {
	var last string
	for last == "" && ctx.Err() == nil {
		id, err := w.src.StatusTail(ctx)
		if err != nil {
			w.pause(ctx, "status tail", err)
			continue
		}
		last = id
	}
	for ctx.Err() == nil {
		evs, next, err := w.src.ReadStatus(ctx, last, w.cfg.Block)
		if err != nil {
			w.pause(ctx, "read status", err)
			continue
		}
		last = next
		w.Handle(ctx, evs)
	}
}

func (w *StatusWatcher) pause(ctx context.Context, what string, err error) {
	if ctx.Err() != nil {
		return
	}
	w.log.Error(what, "err", err)
	select {
	case <-ctx.Done():
	case <-time.After(w.cfg.Backoff):
	}
}

// Handle applies a batch of events. Failures are logged and dropped.
func (w *StatusWatcher) Handle(ctx context.Context, evs []queue.StatusEvent) {
	for _, e := range evs {
		if e.State != queue.StateJudging {
			continue
		}
		if _, err := uuid.Parse(e.SubmissionID); err != nil {
			w.log.Warn("status event with an invalid submission id", "submission", e.SubmissionID)
			continue
		}
		changed, err := w.mk.MarkJudging(ctx, e.SubmissionID)
		switch {
		case err != nil && store.IsPermanent(err):
			w.log.Error("database rejects status event", "submission", e.SubmissionID, "err", err)
		case err != nil:
			w.log.Warn("could not record judging status; dropping", "submission", e.SubmissionID, "err", err)
		case changed:
			w.log.Info("submission judging", "submission", e.SubmissionID, "runner", e.RunnerID)
		}
	}
}
