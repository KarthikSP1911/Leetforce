package server

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"leetforce/api/internal/store"
	"leetforce/queue"
)

// MaxRunInputBytes mirrors the judge engine's custom input limit
// (judge/engine.MaxInputBytes).
const MaxRunInputBytes = 8 << 10

// maxRunBodyBytes allows for JSON escaping of a maximum-size source and input.
const maxRunBodyBytes = 4*(MaxSourceBytes+MaxRunInputBytes) + 4096

// VersionSource returns a problem's current test-set version.
type VersionSource interface {
	TestSetVersion(ctx context.Context, slug string) (string, error)
}

// RunStore keeps the throwaway state of Run jobs. Runs never touch Postgres:
// they are not submissions and must not count towards acceptance.
type RunStore interface {
	SetRun(ctx context.Context, id string, st queue.RunState) error
	GetRun(ctx context.Context, id string) (queue.RunState, bool, error)
}

type runRequest struct {
	Problem  string  `json:"problem"`
	Language string  `json:"language"`
	Source   string  `json:"source"`
	Input    *string `json:"input"` // nil: run the sample tests; set: run once on this input
}

// createRun queues a Run job: the program runs on the problem's sample tests,
// or once on a custom input. Unlike Submit, the result may include input,
// expected and actual output of failing samples and the program's stderr; that
// is safe only because a run never touches hidden tests.
func (d Deps) createRun(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRunBodyBytes)
	var req runRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "body must be JSON with problem, language, source and optional input"})
		return
	}
	switch {
	case req.Problem == "":
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "problem is required"})
		return
	case !slices.Contains(languages, req.Language):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "language must be one of python, cpp, java, go"})
		return
	case req.Source == "":
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "source is required"})
		return
	case len(req.Source) > MaxSourceBytes:
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "source too large"})
		return
	case req.Input != nil && len(*req.Input) > MaxRunInputBytes:
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "input too large"})
		return
	}

	ctx := c.Request.Context()
	version, err := d.Versions.TestSetVersion(ctx, req.Problem)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "problem not found"})
		return
	}
	if err != nil {
		d.fail(c, "run test set version", err)
		return
	}
	if req.Input == nil && len(d.Samples.Samples(req.Problem)) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "problem has no sample tests, supply an input"})
		return
	}

	// The "run-" prefix keeps a run id from ever parsing as a submission id, so a
	// stray run result or dead letter can never be stored as a verdict.
	id := "run-" + uuid.NewString()
	job := queue.Job{SubmissionID: id, Problem: req.Problem, Language: req.Language, Source: req.Source,
		TestSetVersion: version, Kind: queue.KindRun}
	if req.Input != nil {
		job.Custom, job.Input = true, *req.Input
	}
	if err := d.Runs.SetRun(ctx, id, queue.RunState{Status: queue.RunQueued}); err != nil {
		d.fail(c, "set run", err)
		return
	}
	if _, err := d.Queue.Enqueue(ctx, job); err != nil {
		d.Logger.Error("enqueue run", "id", id, "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "could not queue the run, try again"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"id": id, "status": queue.RunQueued})
}

func (d Deps) getRun(c *gin.Context) {
	st, ok, err := d.Runs.GetRun(c.Request.Context(), c.Param("id"))
	if err != nil {
		d.fail(c, "get run", err)
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found or expired"})
		return
	}
	c.JSON(http.StatusOK, st)
}
