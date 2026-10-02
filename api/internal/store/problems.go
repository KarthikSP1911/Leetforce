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
}

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
	rows, err := s.pool.Query(ctx, `SELECT slug, title, difficulty, tags FROM problems ORDER BY slug`)
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
	rows, err := s.pool.Query(ctx, `SELECT slug, title, difficulty, tags FROM problems WHERE slug = $1`, slug)
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
