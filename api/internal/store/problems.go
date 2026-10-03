package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrNotFound means the requested row does not exist.
var ErrNotFound = errors.New("store: not found")

// Problem is a problem's public metadata. The test-set version is internal.
type Problem struct {
	Slug       string   `json:"slug"`
	Title      string   `json:"title"`
	Difficulty string   `json:"difficulty"`
	Tags       []string `json:"tags"`
	// Acceptance is the percentage (0-100) of judged submissions that got AC,
	// or nil when the problem has no judged submissions. Read-only: filled by
	// the list/get queries, ignored by UpsertProblem.
	Acceptance *float64 `json:"acceptance"`
}

// problemSelect computes acceptance from verdicts (internal errors, IE, are not
// the user's fault and are left out of the denominator).
const problemSelect = `
	SELECT p.slug, p.title, p.difficulty, p.tags,
	       (SELECT 100.0 * count(*) FILTER (WHERE v.verdict = 'AC') / NULLIF(count(*), 0)
	          FROM submissions s JOIN verdicts v ON v.submission_id = s.id
	         WHERE s.problem_slug = p.slug AND v.verdict <> 'IE')::float8 AS acceptance
	  FROM problems p`

// UpsertProblem inserts or updates a problem and its current test-set version.
func (s *Store) UpsertProblem(ctx context.Context, p Problem, testSetVersion string) error {
	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO problems (slug, title, difficulty, tags, test_set_version)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (slug) DO UPDATE
		SET title = EXCLUDED.title, difficulty = EXCLUDED.difficulty, tags = EXCLUDED.tags,
		    test_set_version = EXCLUDED.test_set_version, updated_at = now()`,
		p.Slug, p.Title, p.Difficulty, tags, testSetVersion)
	if err != nil {
		return fmt.Errorf("upsert problem %q: %w", p.Slug, err)
	}
	return nil
}

// ListProblems returns all problems ordered by slug.
func (s *Store) ListProblems(ctx context.Context) ([]Problem, error) {
	rows, err := s.pool.Query(ctx, problemSelect+` ORDER BY p.slug`)
	if err != nil {
		return nil, fmt.Errorf("list problems: %w", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Problem])
	if err != nil {
		return nil, fmt.Errorf("list problems: %w", err)
	}
	return out, nil
}

// GetProblem returns one problem, or ErrNotFound.
func (s *Store) GetProblem(ctx context.Context, slug string) (Problem, error) {
	rows, err := s.pool.Query(ctx, problemSelect+` WHERE p.slug = $1`, slug)
	if err != nil {
		return Problem{}, fmt.Errorf("get problem: %w", err)
	}
	p, err := pgx.CollectOneRow(rows, pgx.RowToStructByPos[Problem])
	if errors.Is(err, pgx.ErrNoRows) {
		return Problem{}, ErrNotFound
	}
	if err != nil {
		return Problem{}, fmt.Errorf("get problem: %w", err)
	}
	return p, nil
}

// TestSetVersion returns a problem's current test-set version, or ErrNotFound.
// Run jobs carry it so the runner fetches the same bundle a submission would.
func (s *Store) TestSetVersion(ctx context.Context, slug string) (string, error) {
	var v string
	err := s.pool.QueryRow(ctx, `SELECT test_set_version FROM problems WHERE slug = $1`, slug).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get test set version: %w", err)
	}
	return v, nil
}
