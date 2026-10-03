package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ProblemChange is what SyncProblem found: how the stored test-set version
// compared with the one on disk.
type ProblemChange struct {
	Slug string
	// Created is true when the problem was not in the database before.
	Created bool
	// Old is the version that was stored, empty when Created.
	Old string
	// New is the version now stored.
	New string
}

// VersionChanged reports whether an existing problem moved to a different
// test-set version, the trigger for a rejudge. A brand-new problem has no
// submissions to rejudge.
func (c ProblemChange) VersionChanged() bool { return !c.Created && c.Old != c.New }

// SyncProblem upserts a problem like UpsertProblem and reports whether its
// test-set version changed. The old version is read under a row lock in the
// same transaction as the write, so two API instances starting together
// cannot both see "unchanged" and miss a transition. Old bundles stay in
// object storage: nothing here deletes a version.
func (s *Store) SyncProblem(ctx context.Context, p Problem, testSetVersion string) (ProblemChange, error) {
	ch := ProblemChange{Slug: p.Slug, New: testSetVersion}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ch, fmt.Errorf("sync problem %q: %w", p.Slug, err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	err = tx.QueryRow(ctx, `SELECT test_set_version FROM problems WHERE slug = $1 FOR UPDATE`, p.Slug).Scan(&ch.Old)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		ch.Created = true
	case err != nil:
		return ch, fmt.Errorf("sync problem %q: %w", p.Slug, err)
	}
	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO problems (slug, title, difficulty, tags, test_set_version)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (slug) DO UPDATE
		SET title = EXCLUDED.title, difficulty = EXCLUDED.difficulty, tags = EXCLUDED.tags,
		    test_set_version = EXCLUDED.test_set_version, updated_at = now()`,
		p.Slug, p.Title, p.Difficulty, tags, testSetVersion); err != nil {
		return ch, fmt.Errorf("sync problem %q: %w", p.Slug, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ch, fmt.Errorf("sync problem %q: %w", p.Slug, err)
	}
	return ch, nil
}

// Rejudgeable is a submission taken for rejudging. It carries the source
// because the API reads the database; runners never do.
type Rejudgeable struct {
	ID             string
	Problem        string
	Language       string
	Source         string
	TestSetVersion string
}

// rejudgeWhere selects a problem's submissions that were not judged against
// the problem's current test-set version: judged ones whose verdict names
// another version, and queued or judging ones accepted against an older one.
const rejudgeWhere = `
	s.problem_slug = $1
	AND ((s.status <> 'judged' AND s.test_set_version <> p.test_set_version)
	  OR (s.status = 'judged' AND v.test_set_version IS DISTINCT FROM p.test_set_version))`

// BeginRejudge moves up to limit of a problem's stale submissions onto the
// problem's current test-set version: it sets test_set_version to the current
// one, status back to queued and clears enqueued_at, all in one statement (one
// transaction per batch), and returns them for the caller to enqueue.
//
// A queued or judging submission accepted against an older version also
// qualifies: its in-flight verdict is dropped by RecordVerdict, which stores a
// verdict only for the submission's own version. Once moved, a submission no
// longer qualifies, so repeating the call is safe and a batch loop terminates.
//
// enqueued_at stays NULL until the caller marks the job queued (MarkEnqueued).
// A row whose enqueue fails, or whose process dies, is picked up by the
// reaper, which already re-queues queued rows with no enqueued_at, using the
// new version stored here.
func (s *Store) BeginRejudge(ctx context.Context, slug string, limit int) ([]Rejudgeable, error) {
	rows, err := s.pool.Query(ctx, `
		WITH picked AS (
			SELECT s.id
			FROM submissions s
			JOIN problems p ON p.slug = s.problem_slug
			LEFT JOIN verdicts v ON v.submission_id = s.id
			WHERE `+rejudgeWhere+`
			ORDER BY s.created_at, s.id
			LIMIT $2
			FOR UPDATE OF s SKIP LOCKED
		)
		UPDATE submissions s
		SET test_set_version = p.test_set_version, status = 'queued', enqueued_at = NULL, updated_at = now()
		FROM picked, problems p
		WHERE s.id = picked.id AND p.slug = s.problem_slug
		RETURNING s.id::text, s.problem_slug, s.language, s.source, s.test_set_version`, slug, limit)
	if err != nil {
		return nil, fmt.Errorf("begin rejudge %q: %w", slug, err)
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Rejudgeable, error) {
		var x Rejudgeable
		err := r.Scan(&x.ID, &x.Problem, &x.Language, &x.Source, &x.TestSetVersion)
		return x, err
	})
	if err != nil {
		return nil, fmt.Errorf("begin rejudge %q: %w", slug, err)
	}
	return out, nil
}

// PendingRejudge counts a problem's submissions BeginRejudge would take.
func (s *Store) PendingRejudge(ctx context.Context, slug string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM submissions s
		JOIN problems p ON p.slug = s.problem_slug
		LEFT JOIN verdicts v ON v.submission_id = s.id
		WHERE `+rejudgeWhere, slug).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count rejudge %q: %w", slug, err)
	}
	return n, nil
}
