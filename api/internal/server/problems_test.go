package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"leetforce/api/internal/catalog"
	"leetforce/api/internal/store"
)

type fakeProblems struct {
	list []store.Problem
	err  error
}

func (f fakeProblems) ListProblems(context.Context) ([]store.Problem, error) { return f.list, f.err }

func (f fakeProblems) GetProblem(_ context.Context, slug string) (store.Problem, error) {
	if f.err != nil {
		return store.Problem{}, f.err
	}
	for _, p := range f.list {
		if p.Slug == slug {
			return p, nil
		}
	}
	return store.Problem{}, store.ErrNotFound
}

type fakeSamples map[string][]catalog.Sample

func (f fakeSamples) Samples(slug string) []catalog.Sample { return f[slug] }

func get(t *testing.T, d Deps, path string) *httptest.ResponseRecorder {
	t.Helper()
	d.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	New(d).ServeHTTP(w, req)
	return w
}

func TestProblemEndpoints(t *testing.T) {
	one := store.Problem{Slug: "sum", Title: "Sum", Difficulty: "easy", Tags: []string{"math"}}
	d := Deps{
		Problems: fakeProblems{list: []store.Problem{one}},
		Samples:  fakeSamples{"sum": {{Name: "01", Input: "1 2\n", Expected: "3\n"}}},
	}

	t.Run("list", func(t *testing.T) {
		w := get(t, d, "/problems")
		var body struct{ Problems []store.Problem }
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body.Problems) != 1 || body.Problems[0].Slug != "sum" {
			t.Fatalf("list = %d %s", w.Code, w.Body)
		}
	})
	t.Run("list empty is an array", func(t *testing.T) {
		w := get(t, Deps{Problems: fakeProblems{}}, "/problems")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"problems":[]`) {
			t.Fatalf("empty list = %d %s", w.Code, w.Body)
		}
	})
	t.Run("detail has samples and no test-set version", func(t *testing.T) {
		w := get(t, d, "/problems/sum")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"expected":"3\n"`) {
			t.Fatalf("detail = %d %s", w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), "test_set") {
			t.Fatalf("detail leaks the test-set version: %s", w.Body)
		}
	})
	t.Run("unknown slug is 404", func(t *testing.T) {
		if w := get(t, d, "/problems/nope"); w.Code != 404 {
			t.Fatalf("status = %d", w.Code)
		}
	})
	t.Run("database error is a generic 500", func(t *testing.T) {
		w := get(t, Deps{Problems: fakeProblems{err: errors.New("dial tcp secret-host:5432")}}, "/problems")
		if w.Code != 500 || strings.Contains(w.Body.String(), "secret-host") {
			t.Fatalf("error response = %d %s", w.Code, w.Body)
		}
	})
}
