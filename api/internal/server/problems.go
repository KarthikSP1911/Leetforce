package server

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

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

// ContentSource returns the statement and starter code of a problem. Like
// SampleSource it reads the catalog (files), and never test data.
type ContentSource interface {
	Statement(slug string) string
	Starters(slug string) map[string]string
}

type problemDetail struct {
	store.Problem
	Statement string            `json:"statement"`
	Starters  map[string]string `json:"starters"`
	Samples   []catalog.Sample  `json:"samples"`
}

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// problemItem is a list entry; Solved is true when the signed-in user has an
// accepted submission (always false for anonymous callers).
type problemItem struct {
	store.Problem
	Solved bool `json:"solved"`
}

type problemPage struct {
	Problems []problemItem `json:"problems"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// filterProblems applies ?q= (title substring, case-insensitive),
// ?difficulty= and ?tag= (repeatable; a problem must carry every tag given).
func filterProblems(ps []store.Problem, q, difficulty string, tags []string) []store.Problem {
	q = strings.ToLower(strings.TrimSpace(q))
	out := make([]store.Problem, 0, len(ps))
	for _, p := range ps {
		if q != "" && !strings.Contains(strings.ToLower(p.Title), q) {
			continue
		}
		if difficulty != "" && p.Difficulty != difficulty {
			continue
		}
		if !hasAllTags(p.Tags, tags) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func hasAllTags(have, want []string) bool {
	for _, w := range want {
		if !slices.Contains(have, w) {
			return false
		}
	}
	return true
}

// positiveInt parses an optional query parameter; empty means def.
func positiveInt(raw string, def int) (int, bool) {
	if raw == "" {
		return def, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

func (d Deps) listProblems(c *gin.Context) {
	difficulty := strings.ToLower(c.Query("difficulty"))
	switch difficulty {
	case "", "easy", "medium", "hard":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "difficulty must be easy, medium or hard"})
		return
	}
	page, ok := positiveInt(c.Query("page"), 1)
	size, ok2 := positiveInt(c.Query("page_size"), defaultPageSize)
	if !ok || !ok2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "page and page_size must be positive integers"})
		return
	}
	size = min(size, maxPageSize)

	ps, err := d.Problems.ListProblems(c.Request.Context())
	if err != nil {
		d.fail(c, "list problems", err)
		return
	}
	var tags []string
	for _, t := range c.QueryArray("tag") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	hidden, err := d.hiddenProblems(c)
	if err != nil {
		d.fail(c, "hidden problems", err)
		return
	}
	if len(hidden) > 0 {
		ps = slices.DeleteFunc(ps, func(p store.Problem) bool { return hidden[p.Slug] })
	}
	ps = filterProblems(ps, c.Query("q"), difficulty, tags)
	total := len(ps)
	start := min((page-1)*size, total)
	end := min(start+size, total)
	items := make([]problemItem, 0, end-start)
	var solved map[string]bool
	if user, ok := d.currentUser(c); ok && d.Accounts != nil {
		var err error
		if solved, err = d.Accounts.SolvedProblems(c.Request.Context(), user.ID); err != nil {
			// Solved marks are decoration: list the problems without them.
			d.Logger.Warn("solved problems", "err", err)
		}
	}
	for _, p := range ps[start:end] {
		items = append(items, problemItem{Problem: p, Solved: solved[p.Slug]})
	}
	c.JSON(http.StatusOK, problemPage{Problems: items, Total: total, Page: page, PageSize: size})
}

func (d Deps) getProblem(c *gin.Context) {
	if d.problemHidden(c, c.Param("slug")) {
		return
	}
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
	detail := problemDetail{Problem: p, Starters: map[string]string{}, Samples: samples}
	if d.Content != nil {
		detail.Statement = d.Content.Statement(p.Slug)
		detail.Starters = d.Content.Starters(p.Slug)
	}
	c.JSON(http.StatusOK, detail)
}

// fail logs the real error and returns a generic 500, so internal details
// (hosts, SQL) never reach the client.
func (d Deps) fail(c *gin.Context, what string, err error) {
	d.Logger.Error(what, "err", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}
