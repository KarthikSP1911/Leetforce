package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"leetforce/api/internal/catalog"
	"leetforce/api/internal/store"
)

// ProblemStore reads problem metadata.
type ProblemStore interface {
	ListProblems(ctx context.Context) ([]store.Problem, error)
	GetProblem(ctx context.Context, slug string) (store.Problem, error)
}

// SampleSource returns the visible sample tests of a problem. It is the only
// source of test data the API exposes; hidden tests never pass through it.
type SampleSource interface {
	Samples(slug string) []catalog.Sample
}

type problemDetail struct {
	store.Problem
	Samples []catalog.Sample `json:"samples"`
}

func (d Deps) listProblems(c *gin.Context) {
	ps, err := d.Problems.ListProblems(c.Request.Context())
	if err != nil {
		d.fail(c, "list problems", err)
		return
	}
	if ps == nil {
		ps = []store.Problem{}
	}
	c.JSON(http.StatusOK, gin.H{"problems": ps})
}

func (d Deps) getProblem(c *gin.Context) {
	p, err := d.Problems.GetProblem(c.Request.Context(), c.Param("slug"))
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "problem not found"})
		return
	}
	if err != nil {
		d.fail(c, "get problem", err)
		return
	}
	samples := d.Samples.Samples(p.Slug)
	if samples == nil {
		samples = []catalog.Sample{}
	}
	c.JSON(http.StatusOK, problemDetail{Problem: p, Samples: samples})
}

// fail logs the real error and returns a generic 500, so internal details
// (hosts, SQL) never reach the client.
func (d Deps) fail(c *gin.Context, what string, err error) {
	d.Logger.Error(what, "err", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}
