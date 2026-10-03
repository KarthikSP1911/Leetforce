package contest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is what the handlers and Phase 15's leaderboard read.
type Store interface {
	// ListContests returns every contest, newest start first. userID may be
	// empty (anonymous); it only fills Registered.
	ListContests(ctx context.Context, userID string) ([]Contest, error)
	// GetContest returns one contest by slug, or ErrNotFound.
	GetContest(ctx context.Context, slug, userID string) (Contest, error)
	// Events returns the judged contest submissions that count towards the
	// standings: submissions tagged with the contest, inside its window.
	Events(ctx context.Context, contestID string) ([]Event, error)
	IsRegistered(ctx context.Context, contestID, userID string) (bool, error)
	Register(ctx context.Context, contestID, userID string) error
	Problems(ctx context.Context, contestID string) ([]Problem, error)
}

// PG is the Postgres implementation of Store and of the submit gate the server
// uses (HiddenProblems, CheckSubmit, SetSubmissionContest).
type PG struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewPG returns a store over the API's pool.
func NewPG(pool *pgxpool.Pool) *PG { return &PG{pool: pool, now: time.Now} }

const contestSelect = `
	SELECT c.id::text, c.slug, c.title, c.starts_at, c.ends_at,
	       (SELECT count(*) FROM contest_problems cp WHERE cp.contest_id = c.id)::int,
	       EXISTS (SELECT 1 FROM contest_participants p
	                WHERE p.contest_id = c.id AND p.user_id = NULLIF($1, '')::uuid)
	  FROM contests c`

func (s *PG) scan(row pgx.Row) (Contest, error) {
	var c Contest
	if err := row.Scan(&c.ID, &c.Slug, &c.Title, &c.StartsAt, &c.EndsAt, &c.ProblemCount, &c.Registered); err != nil {
		return Contest{}, err
	}
	c.Status = StatusAt(c.StartsAt, c.EndsAt, s.now())
	return c, nil
}

func (s *PG) ListContests(ctx context.Context, userID string) ([]Contest, error) {
	rows, err := s.pool.Query(ctx, contestSelect+` ORDER BY c.starts_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list contests: %w", err)
	}
	defer rows.Close()
	out := []Contest{}
	for rows.Next() {
		c, err := s.scan(rows)
		if err != nil {
			return nil, fmt.Errorf("list contests: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list contests: %w", err)
	}
	return out, nil
}

func (s *PG) GetContest(ctx context.Context, slug, userID string) (Contest, error) {
	c, err := s.scan(s.pool.QueryRow(ctx, contestSelect+` WHERE c.slug = $2`, userID, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return Contest{}, ErrNotFound
	}
	if err != nil {
		return Contest{}, fmt.Errorf("get contest: %w", err)
	}
	return c, nil
}

func (s *PG) Events(ctx context.Context, contestID string) ([]Event, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT s.user_id::text, s.problem_slug, s.created_at, s.created_at - c.starts_at, v.verdict
		  FROM submissions s
		  JOIN contests c ON c.id = s.contest_id
		  JOIN verdicts v ON v.submission_id = s.id
		 WHERE s.contest_id = $1::uuid AND s.user_id IS NOT NULL
		   AND s.created_at >= c.starts_at AND s.created_at < c.ends_at
		 ORDER BY s.created_at`, contestID)
	if err != nil {
		return nil, fmt.Errorf("contest events: %w", err)
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.UserID, &e.ProblemSlug, &e.SubmittedAt, &e.Elapsed, &e.Verdict); err != nil {
			return nil, fmt.Errorf("contest events: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contest events: %w", err)
	}
	return out, nil
}

func (s *PG) IsRegistered(ctx context.Context, contestID, userID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM contest_participants
		                WHERE contest_id = $1::uuid AND user_id = NULLIF($2, '')::uuid)`,
		contestID, userID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("is registered: %w", err)
	}
	return ok, nil
}

func (s *PG) Register(ctx context.Context, contestID, userID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO contest_participants (contest_id, user_id) VALUES ($1::uuid, $2::uuid)
		ON CONFLICT DO NOTHING`, contestID, userID)
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}
	return nil
}

func (s *PG) Problems(ctx context.Context, contestID string) ([]Problem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT cp.label, cp.position, cp.points, p.slug, p.title, p.difficulty
		  FROM contest_problems cp JOIN problems p ON p.slug = cp.problem_slug
		 WHERE cp.contest_id = $1::uuid
		 ORDER BY cp.position`, contestID)
	if err != nil {
		return nil, fmt.Errorf("contest problems: %w", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Problem])
	if err != nil {
		return nil, fmt.Errorf("contest problems: %w", err)
	}
	return out, nil
}

// HiddenProblems returns the problems userID (empty for anonymous) may not see:
// those of a contest that has not started, or that is running and the user is
// not registered for. Once a contest has ended its problems are public.
func (s *PG) HiddenProblems(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT cp.problem_slug
		  FROM contest_problems cp JOIN contests c ON c.id = cp.contest_id
		 WHERE c.ends_at > $2
		   AND (c.starts_at > $2 OR NOT EXISTS (
		         SELECT 1 FROM contest_participants p
		          WHERE p.contest_id = c.id AND p.user_id = NULLIF($1, '')::uuid))`,
		userID, s.now())
	if err != nil {
		return nil, fmt.Errorf("hidden problems: %w", err)
	}
	slugs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("hidden problems: %w", err)
	}
	out := make(map[string]bool, len(slugs))
	for _, sl := range slugs {
		out[sl] = true
	}
	return out, nil
}

// CheckSubmit decides whether userID may submit problem to the contest named by
// ref (a slug or id) now, and returns the contest's id. Errors: ErrNotFound,
// ErrClosed (outside the window), ErrNotRegistered, ErrNotInContest.
func (s *PG) CheckSubmit(ctx context.Context, ref, userID, problem string) (string, error) {
	var c Contest
	var err error
	if c, err = s.GetContest(ctx, ref, userID); errors.Is(err, ErrNotFound) {
		// Also accept the id, so a client that holds only the id still works.
		c, err = s.scan(s.pool.QueryRow(ctx, contestSelect+` WHERE c.id::text = $2`, userID, ref))
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
	}
	if err != nil {
		return "", err
	}
	if c.Status != StatusRunning {
		return "", ErrClosed
	}
	if !c.Registered {
		return "", ErrNotRegistered
	}
	var in bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM contest_problems WHERE contest_id = $1::uuid AND problem_slug = $2)`,
		c.ID, problem).Scan(&in); err != nil {
		return "", fmt.Errorf("check contest problem: %w", err)
	}
	if !in {
		return "", ErrNotInContest
	}
	return c.ID, nil
}

// SetSubmissionContest tags a submission with its contest.
func (s *PG) SetSubmissionContest(ctx context.Context, submissionID, contestID string) error {
	if _, err := s.pool.Exec(ctx, `UPDATE submissions SET contest_id = $2::uuid WHERE id = $1::uuid`,
		submissionID, contestID); err != nil {
		return fmt.Errorf("set submission contest: %w", err)
	}
	return nil
}
