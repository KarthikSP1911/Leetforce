package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"leetforce/api/internal/leaderboard"
)

// The queries below read the Phase 14 tables (contests, contest_problems,
// contest_participants, submissions.contest_id). Column names are the Phase 15
// assumptions listed in ADR 0025; Phase 16 checks them against migration 14.

// ContestBySlug returns a contest and its problems in contest order, or
// leaderboard.ErrNotFound.
func (s *Store) ContestBySlug(ctx context.Context, slug string) (leaderboard.Contest, error) {
	var c leaderboard.Contest
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, slug, title, starts_at, ends_at FROM contests WHERE slug = $1`, slug).
		Scan(&c.ID, &c.Slug, &c.Title, &c.StartsAt, &c.EndsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return leaderboard.Contest{}, leaderboard.ErrNotFound
	}
	if err != nil {
		return leaderboard.Contest{}, fmt.Errorf("contest by slug: %w", err)
	}
	rows, err := s.pool.Query(ctx,
		`SELECT problem_slug FROM contest_problems WHERE contest_id = $1::uuid ORDER BY position, problem_slug`, c.ID)
	if err != nil {
		return leaderboard.Contest{}, fmt.Errorf("contest problems: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return leaderboard.Contest{}, fmt.Errorf("contest problems: %w", err)
		}
		c.Problems = append(c.Problems, p)
	}
	return c, rows.Err()
}

// ContestEvents returns the judged submissions of a contest's registered
// participants on the contest's problems, and the participants' usernames.
// Submissions without a verdict yet are not events.
func (s *Store) ContestEvents(ctx context.Context, contestID string) ([]leaderboard.Event, map[string]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT s.user_id::text, u.username, s.problem_slug, s.created_at, v.verdict
		FROM submissions s
		JOIN verdicts v ON v.submission_id = s.id
		JOIN contest_participants cp ON cp.contest_id = s.contest_id AND cp.user_id = s.user_id
		JOIN users u ON u.id = s.user_id
		WHERE s.contest_id = $1::uuid`, contestID)
	if err != nil {
		return nil, nil, fmt.Errorf("contest events: %w", err)
	}
	defer rows.Close()
	var out []leaderboard.Event
	names := map[string]string{}
	for rows.Next() {
		var e leaderboard.Event
		var name string
		if err := rows.Scan(&e.UserID, &name, &e.ProblemSlug, &e.SubmittedAt, &e.Verdict); err != nil {
			return nil, nil, fmt.Errorf("contest events: %w", err)
		}
		names[e.UserID] = name
		out = append(out, e)
	}
	return out, names, rows.Err()
}

// GlobalRows counts, per user, the distinct problems with at least one AC, by
// difficulty, and the time of the latest first-AC.
func (s *Store) GlobalRows(ctx context.Context) ([]leaderboard.GlobalRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id::text, u.username,
		       count(*) FILTER (WHERE p.difficulty = 'easy'),
		       count(*) FILTER (WHERE p.difficulty = 'medium'),
		       count(*) FILTER (WHERE p.difficulty = 'hard'),
		       max(a.first_ac)
		FROM (
			SELECT s.user_id, s.problem_slug, min(s.created_at) AS first_ac
			FROM submissions s JOIN verdicts v ON v.submission_id = s.id
			WHERE v.verdict = 'AC' AND s.user_id IS NOT NULL
			GROUP BY s.user_id, s.problem_slug
		) a
		JOIN problems p ON p.slug = a.problem_slug
		JOIN users u ON u.id = a.user_id
		GROUP BY u.id, u.username`)
	if err != nil {
		return nil, fmt.Errorf("global rows: %w", err)
	}
	defer rows.Close()
	var out []leaderboard.GlobalRow
	for rows.Next() {
		var r leaderboard.GlobalRow
		var last *time.Time
		if err := rows.Scan(&r.UserID, &r.Username, &r.Easy, &r.Medium, &r.Hard, &last); err != nil {
			return nil, fmt.Errorf("global rows: %w", err)
		}
		if last != nil {
			r.LastAC = *last
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SubmissionContest returns the contest a submission was made in, if any.
func (s *Store) SubmissionContest(ctx context.Context, submissionID string) (string, bool, error) {
	var id *string
	err := s.pool.QueryRow(ctx,
		`SELECT contest_id::text FROM submissions WHERE id = $1::uuid`, submissionID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && id == nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("submission contest: %w", err)
	}
	return *id, true, nil
}
