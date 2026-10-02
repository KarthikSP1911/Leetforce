package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Submission statuses.
const (
	StatusQueued  = "queued"
	StatusJudging = "judging"
	StatusJudged  = "judged"
)

// Submission is what the API returns about a submission: its state and, once
// judged, the verdict. It never carries the source, test data or stderr.
type Submission struct {
	ID             string       `json:"id"`
	Problem        string       `json:"problem"`
	Language       string       `json:"language"`
	Status         string       `json:"status"`
	TestSetVersion string       `json:"-"`
	CreatedAt      time.Time    `json:"created_at"`
	Verdict        *VerdictView `json:"verdict,omitempty"`
}

// VerdictView is the part of a verdict a user may see for Submit.
type VerdictView struct {
	Verdict   string `json:"verdict"`
	RuntimeMS int64  `json:"runtime_ms"`
	MemoryKB  int64  `json:"memory_kb"`
	Passed    int    `json:"passed"`
	Total     int    `json:"total"`
}

// InsertSubmission records a queued submission and stamps it with the
// problem's current test-set version in the same statement, so the version
// cannot change between the check and the insert, and returns that version so
// the job can name it. It returns ErrNotFound if the problem does not exist.
func (s *Store) InsertSubmission(ctx context.Context, id, problem, language, source string) (string, error) {
	var version string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO submissions (id, problem_slug, language, source, test_set_version)
		SELECT $1, slug, $3, $4, test_set_version FROM problems WHERE slug = $2
		RETURNING test_set_version`,
		id, problem, language, source).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("insert submission: %w", err)
	}
	return version, nil
}

// DeleteSubmission removes a submission that could not be queued.
func (s *Store) DeleteSubmission(ctx context.Context, id string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM submissions WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete submission: %w", err)
	}
	return nil
}

// GetSubmission returns a submission with its verdict if there is one, or
// ErrNotFound (also for an id that is not a UUID).
func (s *Store) GetSubmission(ctx context.Context, id string) (Submission, error) {
	var sub Submission
	var v struct {
		Verdict   *string
		RuntimeMS *int64
		MemoryKB  *int64
		Passed    *int
		Total     *int
	}
	err := s.pool.QueryRow(ctx, `
		SELECT s.id::text, s.problem_slug, s.language, s.status, s.test_set_version, s.created_at,
		       v.verdict, v.runtime_ms, v.memory_kb, v.passed, v.total
		FROM submissions s LEFT JOIN verdicts v ON v.submission_id = s.id
		WHERE s.id::text = $1`, id).
		Scan(&sub.ID, &sub.Problem, &sub.Language, &sub.Status, &sub.TestSetVersion, &sub.CreatedAt,
			&v.Verdict, &v.RuntimeMS, &v.MemoryKB, &v.Passed, &v.Total)
	if errors.Is(err, pgx.ErrNoRows) {
		return Submission{}, ErrNotFound
	}
	if err != nil {
		return Submission{}, fmt.Errorf("get submission: %w", err)
	}
	if v.Verdict != nil {
		sub.Verdict = &VerdictView{Verdict: *v.Verdict, RuntimeMS: *v.RuntimeMS, MemoryKB: *v.MemoryKB, Passed: *v.Passed, Total: *v.Total}
	}
	return sub, nil
}

// MarkJudging records that a runner took the submission's job. It only moves a
// queued submission forward: a judged one (or one already judging) is left
// alone, so a late or repeated event can never undo a verdict. It returns true
// if the status changed.
func (s *Store) MarkJudging(ctx context.Context, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE submissions SET status = 'judging', updated_at = now()
		WHERE id = $1::uuid AND status = 'queued'`, id)
	if err != nil {
		return false, fmt.Errorf("mark judging %s: %w", id, err)
	}
	return tag.RowsAffected() == 1, nil
}
