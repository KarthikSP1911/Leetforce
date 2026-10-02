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

// MaxSourceBytes mirrors the judge engine's limit (judge/engine.MaxSourceBytes)
// so an oversized submission is rejected here instead of becoming an IE job.
const MaxSourceBytes = 64 << 10

// maxBodyBytes allows for JSON escaping of a maximum-size source.
const maxBodyBytes = 4*MaxSourceBytes + 4096

var languages = []string{"python", "cpp", "java", "go"}

// SubmissionStore records submissions.
type SubmissionStore interface {
	InsertSubmission(ctx context.Context, id, problem, language, source string) (testSetVersion string, err error)
	DeleteSubmission(ctx context.Context, id string) error
	MarkEnqueued(ctx context.Context, id string) error
	GetSubmission(ctx context.Context, id string) (store.Submission, error)
}

// Enqueuer puts a job on the queue for the runners.
type Enqueuer interface {
	Enqueue(ctx context.Context, j queue.Job) (string, error)
}

type submitRequest struct {
	Problem  string `json:"problem"`
	Language string `json:"language"`
	Source   string `json:"source"`
}

func (d Deps) createSubmission(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	var req submitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "body must be JSON with problem, language and source"})
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
	}

	ctx := c.Request.Context()
	id := uuid.NewString()
	version, err := d.Submissions.InsertSubmission(ctx, id, req.Problem, req.Language, req.Source)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "problem not found"})
			return
		}
		d.fail(c, "insert submission", err)
		return
	}
	job := queue.Job{SubmissionID: id, Problem: req.Problem, Language: req.Language, Source: req.Source, TestSetVersion: version}
	if _, err := d.Queue.Enqueue(ctx, job); err != nil {
		// Do not leave a row that no runner will ever see. Use a fresh context:
		// the request's may be the thing that failed.
		d.Logger.Error("enqueue submission", "id", id, "err", err)
		if delErr := d.Submissions.DeleteSubmission(context.WithoutCancel(ctx), id); delErr != nil {
			d.Logger.Error("remove unqueued submission", "id", id, "err", delErr)
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "could not queue the submission, try again"})
		return
	}
	// Without this mark the reaper would queue the job a second time after its
	// grace period. That is harmless (a verdict is stored once), so a failure
	// here is logged and the submission is still accepted.
	if err := d.Submissions.MarkEnqueued(context.WithoutCancel(ctx), id); err != nil {
		d.Logger.Warn("mark submission enqueued", "id", id, "err", err)
	}
	c.JSON(http.StatusAccepted, gin.H{"id": id, "status": store.StatusQueued})
}

func (d Deps) getSubmission(c *gin.Context) {
	sub, err := d.Submissions.GetSubmission(c.Request.Context(), c.Param("id"))
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		return
	}
	if err != nil {
		d.fail(c, "get submission", err)
		return
	}
	c.JSON(http.StatusOK, sub)
}
