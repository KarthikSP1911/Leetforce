// Package leaderboard computes contest standings and the global ranking and
// caches them in Redis. Both are pure functions of rows in Postgres: the cache
// only saves recomputation, so it can never hold a ranking the database would
// not give (see ADR 0025).
package leaderboard

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound means the contest does not exist.
var ErrNotFound = errors.New("leaderboard: not found")

// Event is one judged submission inside a contest. It mirrors the Phase 14
// contract (contest.Event) so this package builds without it.
type Event struct {
	UserID      string
	ProblemSlug string
	SubmittedAt time.Time
	Verdict     string
}

// Standing is the part of contest.Standing this package uses.
type Standing struct {
	UserID         string
	Solved         int
	PenaltyMinutes int
}

// Scorer applies the ICPC rules (contest.Score). It receives events whose
// SubmittedAt is rebased to "time since the contest start, as an offset from
// the Unix epoch"; see scoreEvents.
type Scorer func(events []Event) []Standing

// Contest is the metadata standings need.
type Contest struct {
	ID       string
	Slug     string
	Title    string
	StartsAt time.Time
	EndsAt   time.Time
	Problems []string // slugs, in contest order
}

// GlobalRow is one user's solved counts per difficulty, from SQL.
type GlobalRow struct {
	UserID   string
	Username string
	Easy     int
	Medium   int
	Hard     int
	LastAC   time.Time // time of the most recent first-AC
}

// Source is the database side.
type Source interface {
	ContestBySlug(ctx context.Context, slug string) (Contest, error)
	// ContestEvents returns every judged submission of the contest's
	// participants (any time; the window is applied here) and their usernames.
	ContestEvents(ctx context.Context, contestID string) ([]Event, map[string]string, error)
	GlobalRows(ctx context.Context) ([]GlobalRow, error)
	// SubmissionContest returns the contest a submission belongs to, if any.
	SubmissionContest(ctx context.Context, submissionID string) (string, bool, error)
}

// Cache is the Redis side (queue.Queue implements it).
type Cache interface {
	KVGet(ctx context.Context, key string) ([]byte, bool, error)
	KVSet(ctx context.Context, key string, val []byte, ttl time.Duration) error
	KVIncr(ctx context.Context, key string) (int64, error)
	KVCounter(ctx context.Context, key string) (int64, error)
}

// Cell is one participant's result on one problem.
type Cell struct {
	Solved   bool `json:"solved"`
	Attempts int  `json:"attempts"` // rejected attempts before the first AC (all, if unsolved)
	Minutes  int  `json:"minutes"`  // minutes from the start to the first AC
}

// StandingRow is one line of a contest scoreboard.
type StandingRow struct {
	Rank           int             `json:"rank"`
	UserID         string          `json:"user_id"`
	Username       string          `json:"username"`
	Solved         int             `json:"solved"`
	PenaltyMinutes int             `json:"penalty_minutes"`
	LastACAt       *time.Time      `json:"last_ac_at"`
	Cells          map[string]Cell `json:"cells"`
}

// ContestInfo is the contest header in a standings response.
type ContestInfo struct {
	Slug     string    `json:"slug"`
	Title    string    `json:"title"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

// ContestStandings is the GET /contests/:slug/standings body.
type ContestStandings struct {
	Contest     ContestInfo   `json:"contest"`
	GeneratedAt time.Time     `json:"generated_at"`
	Problems    []string      `json:"problems"`
	Standings   []StandingRow `json:"standings"`
}

// GlobalEntry is one line of the global ranking.
type GlobalEntry struct {
	Rank     int        `json:"rank"`
	UserID   string     `json:"user_id"`
	Username string     `json:"username"`
	Solved   int        `json:"solved"`
	Easy     int        `json:"easy"`
	Medium   int        `json:"medium"`
	Hard     int        `json:"hard"`
	Score    int        `json:"score"`
	LastACAt *time.Time `json:"last_ac_at"`
}

// GlobalPage is the GET /leaderboard body.
type GlobalPage struct {
	Page        int           `json:"page"`
	PerPage     int           `json:"per_page"`
	Total       int           `json:"total"`
	GeneratedAt time.Time     `json:"generated_at"`
	Entries     []GlobalEntry `json:"entries"`
}
