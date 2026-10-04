// Package contest owns timed contests: the model, the Postgres store, ICPC
// scoring and the HTTP handlers.
package contest

import (
	"errors"
	"time"
)

// Contest statuses, derived from the clock and never stored.
const (
	StatusUpcoming = "upcoming"
	StatusRunning  = "running"
	StatusEnded    = "ended"
)

// Errors the store and the submit gate return.
var (
	ErrNotFound      = errors.New("contest: not found")
	ErrClosed        = errors.New("contest: window is not open")
	ErrNotRegistered = errors.New("contest: not registered")
	ErrNotInContest  = errors.New("contest: problem is not part of this contest")
)

// Contest is a timed window over a set of problems.
type Contest struct {
	ID           string    `json:"-"`
	Slug         string    `json:"slug"`
	Title        string    `json:"title"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
	Status       string    `json:"status"`
	Registered   bool      `json:"registered"`
	ProblemCount int       `json:"problem_count"`
}

// StatusAt returns the contest's status at time t. The window is [start, end).
func StatusAt(start, end, t time.Time) string {
	switch {
	case t.Before(start):
		return StatusUpcoming
	case t.Before(end):
		return StatusRunning
	default:
		return StatusEnded
	}
}

// Problem is a contest problem with its label (A, B, ...) and metadata.
type Problem struct {
	Label      string `json:"label"`
	Position   int    `json:"position"`
	Points     int    `json:"points"`
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	Difficulty string `json:"difficulty"`
}
