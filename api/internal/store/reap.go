package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Unqueued is a submission that was stored but whose job never reached the
// queue. It carries what the job needs, including the test-set version the
// submission was accepted against.
type Unqueued struct {
	ID             string
	Problem        string
	Language       string
	Source         string
	TestSetVersion string
}

// MarkEnqueued records that the submission's job is on the queue.
func (s *Store) MarkEnqueued(ctx context.Context, id string) error {
	if _, err := s.pool.Exec(ctx, `
		UPDATE submissions SET enqueued_at = now() WHERE id = $1::uuid AND enqueued_at IS NULL`, id); err != nil {
		return fmt.Errorf("mark enqueued %s: %w", id, err)
	}
	return nil
}

// ReapUnqueued calls enqueue for up to limit queued submissions that are older
// than grace and still have no enqueued_at, and marks each one whose enqueue
// succeeded. The rows are locked with FOR UPDATE SKIP LOCKED inside one
// transaction, so several API instances never take the same row, and a crash
// before the commit rolls the marks back (the job may then be queued twice,
// which is harmless: a runner skips a job whose verdict exists and the verdict
// write is idempotent). It returns how many were queued and the joined errors
// of the ones that failed; a failed row stays unmarked for the next sweep.
func (s *Store) ReapUnqueued(ctx context.Context, grace time.Duration, limit int, enqueue func(context.Context, Unqueued) error) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("reap unqueued: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	rows, err := tx.Query(ctx, `
		SELECT id::text, problem_slug, language, source, test_set_version
		FROM submissions
		WHERE status = 'queued' AND enqueued_at IS NULL
		  AND created_at < now() - make_interval(secs => $1)
		ORDER BY created_at
		LIMIT $2
		FOR UPDATE SKIP LOCKED`, grace.Seconds(), limit)
	if err != nil {
		return 0, fmt.Errorf("reap unqueued: %w", err)
	}
	var found []Unqueued
	for rows.Next() {
		var u Unqueued
		if err := rows.Scan(&u.ID, &u.Problem, &u.Language, &u.Source, &u.TestSetVersion); err != nil {
			rows.Close()
			return 0, fmt.Errorf("reap unqueued: %w", err)
		}
		found = append(found, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("reap unqueued: %w", err)
	}

	var done []string
	var errs []error
	for _, u := range found {
		if err := enqueue(ctx, u); err != nil {
			errs = append(errs, fmt.Errorf("requeue %s: %w", u.ID, err))
			continue
		}
		done = append(done, u.ID)
	}
	if len(done) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE submissions SET enqueued_at = now() WHERE id = ANY($1::uuid[])`, done); err != nil {
			return 0, errors.Join(append(errs, fmt.Errorf("reap unqueued: %w", err))...)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, errors.Join(append(errs, fmt.Errorf("reap unqueued: %w", err))...)
	}
	return len(done), errors.Join(errs...)
}
