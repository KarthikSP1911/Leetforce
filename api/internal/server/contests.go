package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"leetforce/api/internal/contest"
)

// ContestService is the contest store plus the checks that gate problems and
// submissions. *contest.PG implements it. A nil Deps.Contests turns contests
// off: no routes, no hidden problems.
type ContestService interface {
	contest.Store
	HiddenProblems(ctx context.Context, userID string) (map[string]bool, error)
	CheckSubmit(ctx context.Context, ref, userID, problem string) (contestID string, err error)
	SetSubmissionContest(ctx context.Context, submissionID, contestID string) error
}

// contestRoutes registers the /contests endpoints.
func (d Deps) contestRoutes(r gin.IRoutes) {
	if d.Contests == nil {
		return
	}
	contest.Handler{
		Store:  d.Contests,
		Logger: d.Logger,
		UserID: func(c *gin.Context) (string, bool) {
			u, ok := d.currentUser(c)
			return u.ID, ok
		},
		RequireUser: func(c *gin.Context) (string, bool) {
			u, ok := d.requireUser(c)
			return u.ID, ok
		},
	}.Routes(r)
}

// hiddenProblems returns the problems the caller may not see yet (contest-only
// problems before the start, or during the contest for unregistered users).
// With contests off it is empty. An error is returned, not swallowed: failing
// open would leak problems.
func (d Deps) hiddenProblems(c *gin.Context) (map[string]bool, error) {
	if d.Contests == nil {
		return nil, nil
	}
	userID := ""
	if u, ok := d.currentUser(c); ok {
		userID = u.ID
	}
	return d.Contests.HiddenProblems(c.Request.Context(), userID)
}

// problemHidden answers 404 and returns true when slug is hidden from the caller.
func (d Deps) problemHidden(c *gin.Context, slug string) bool {
	hidden, err := d.hiddenProblems(c)
	if err != nil {
		d.fail(c, "hidden problems", err)
		return true
	}
	if hidden[slug] {
		c.JSON(http.StatusNotFound, gin.H{"error": "problem not found"})
		return true
	}
	return false
}

// contestSubmit checks a submission that names a contest. It answers the error
// itself and returns false when the submission must be refused: 404 for an
// unknown contest, 409 when the window is closed or the user is not registered,
// 422 when the problem is not in the contest.
func (d Deps) contestSubmit(c *gin.Context, ref, userID, problem string) (string, bool) {
	if d.Contests == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "contest not found"})
		return "", false
	}
	id, err := d.Contests.CheckSubmit(c.Request.Context(), ref, userID, problem)
	switch {
	case err == nil:
		return id, true
	case errors.Is(err, contest.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "contest not found"})
	case errors.Is(err, contest.ErrClosed):
		c.JSON(http.StatusConflict, gin.H{"error": "the contest is not open"})
	case errors.Is(err, contest.ErrNotRegistered):
		c.JSON(http.StatusConflict, gin.H{"error": "register for the contest first"})
	case errors.Is(err, contest.ErrNotInContest):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "problem is not part of this contest"})
	default:
		d.fail(c, "check contest submit", err)
	}
	return "", false
}
