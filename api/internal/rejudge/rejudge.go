// Package rejudge re-queues submissions that were judged against an older
// test-set version of their problem.
//
// When a problem's tests are fixed the content-based test-set version changes
// (ADR 0005). The store moves each stale submission onto the new version and
// back to queued (store.BeginRejudge, one transaction per batch), this
// package then puts a job with the new version on the queue and marks the row
// enqueued. A row whose enqueue fails stays unmarked and the reaper re-queues
// it, so a crash or a Redis outage in the middle loses nothing. The new
// verdict replaces the old one through store.RecordVerdict.
package rejudge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"leetforce/api/internal/store"
	"leetforce/judge/problem"
	"leetforce/queue"
)

// DefaultBatch is how many submissions one transaction takes.
const DefaultBatch = 100

// Store is the database side.
type Store interface {
	BeginRejudge(ctx context.Context, slug string, limit int) ([]store.Rejudgeable, error)
	MarkEnqueued(ctx context.Context, id string) error
}

// Enqueuer puts a job on the queue.
type Enqueuer interface {
	Enqueue(ctx context.Context, j queue.Job) (string, error)
}

// Result counts what one Run did.
type Result struct {
	// Requeued is how many submissions were moved to the new version and
	// enqueued.
	Requeued int
	// Deferred is how many were moved but not enqueued (or not marked); the
	// reaper picks them up.
	Deferred int
}

// Rejudger runs rejudges.
type Rejudger struct {
	st    Store
	q     Enqueuer
	log   *slog.Logger
	batch int
}

// New builds a Rejudger. batch <= 0 means DefaultBatch.
func New(st Store, q Enqueuer, log *slog.Logger, batch int) *Rejudger {
	if batch <= 0 {
		batch = DefaultBatch
	}
	return &Rejudger{st: st, q: q, log: log, batch: batch}
}

// Run rejudges every stale submission of one problem, batch by batch. It is
// idempotent: running it again after it finished finds nothing to do, and
// running it twice at once is safe because each batch locks its rows with
// SKIP LOCKED. It stops at the first database error; enqueue failures are
// counted as Deferred and do not stop the run.
func (r *Rejudger) Run(ctx context.Context, slug string) (Result, error) {
	var res Result
	var errs []error
	for {
		if err := ctx.Err(); err != nil {
			return res, errors.Join(append(errs, err)...)
		}
		batch, err := r.st.BeginRejudge(ctx, slug, r.batch)
		if err != nil {
			return res, errors.Join(append(errs, err)...)
		}
		for _, s := range batch {
			if err := r.enqueue(ctx, s); err != nil {
				res.Deferred++
				errs = append(errs, err)
				r.log.Warn("rejudge enqueue failed; the reaper will retry", "submission", s.ID, "err", err)
				continue
			}
			res.Requeued++
		}
		if len(batch) < r.batch {
			return res, errors.Join(errs...)
		}
	}
}

func (r *Rejudger) enqueue(ctx context.Context, s store.Rejudgeable) error {
	if _, err := r.q.Enqueue(ctx, queue.Job{
		SubmissionID: s.ID, Problem: s.Problem, Language: s.Language,
		Source: s.Source, TestSetVersion: s.TestSetVersion,
	}); err != nil {
		return fmt.Errorf("enqueue rejudge %s: %w", s.ID, err)
	}
	if err := r.st.MarkEnqueued(ctx, s.ID); err != nil {
		// The job is on the queue; a second one from the reaper is harmless
		// (the runner skips a job whose verdict exists, the write is idempotent).
		return fmt.Errorf("mark rejudge %s enqueued: %w", s.ID, err)
	}
	return nil
}

// Syncer is the database side of publishing problems.
type Syncer interface {
	SyncProblem(ctx context.Context, p store.Problem, testSetVersion string) (store.ProblemChange, error)
}

// Sync upserts every problem and returns what happened to each one, in order.
// Call RunChanged with the result once the bundles are published and the queue
// is open.
func Sync(ctx context.Context, st Syncer, problems []*problem.Problem) ([]store.ProblemChange, error) {
	changes := make([]store.ProblemChange, 0, len(problems))
	for _, p := range problems {
		sp := store.Problem{Slug: p.Spec.Slug, Title: p.Spec.Title, Difficulty: p.Spec.Difficulty, Tags: p.Spec.Tags}
		ch, err := st.SyncProblem(ctx, sp, p.TestSetVer)
		if err != nil {
			return changes, err
		}
		changes = append(changes, ch)
	}
	return changes, nil
}

// RunChanged rejudges each problem whose version changed and logs the counts.
// Failures are logged per problem and joined; one bad problem does not stop
// the others.
func (r *Rejudger) RunChanged(ctx context.Context, changes []store.ProblemChange) error {
	var errs []error
	for _, c := range changes {
		if !c.VersionChanged() {
			continue
		}
		res, err := r.Run(ctx, c.Slug)
		r.log.Info("problem test set changed; submissions rejudged",
			"problem", c.Slug, "old_version", c.Old, "new_version", c.New,
			"requeued", res.Requeued, "deferred", res.Deferred, "err", err)
		if err != nil {
			errs = append(errs, fmt.Errorf("rejudge %s: %w", c.Slug, err))
		}
	}
	return errors.Join(errs...)
}
