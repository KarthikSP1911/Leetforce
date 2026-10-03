package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"leetforce/api/internal/store"
	"leetforce/queue"
)

type fakeVersions struct{ missing bool }

func (f fakeVersions) TestSetVersion(context.Context, string) (string, error) {
	if f.missing {
		return "", store.ErrNotFound
	}
	return fakeVersion, nil
}

type fakeRuns struct{ m map[string]queue.RunState }

func (f *fakeRuns) SetRun(_ context.Context, id string, st queue.RunState) error {
	if f.m == nil {
		f.m = map[string]queue.RunState{}
	}
	f.m[id] = st
	return nil
}

func (f *fakeRuns) GetRun(_ context.Context, id string) (queue.RunState, bool, error) {
	st, ok := f.m[id]
	return st, ok, nil
}

func runDeps(q *fakeQueue, runs *fakeRuns, v fakeVersions) Deps {
	return Deps{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Queue:    q,
		Runs:     runs,
		Versions: v,
		Samples:  fakeSamples{"sample-sum": {{Name: "1", Input: "1 2", Expected: "3"}}},
	}
}

func postRun(d Deps, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/runs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	New(d).ServeHTTP(w, req)
	return w
}

func TestCreateRun(t *testing.T) {
	t.Run("sample run is queued as a run job and never as a submission", func(t *testing.T) {
		q, runs := &fakeQueue{}, &fakeRuns{}
		w := postRun(runDeps(q, runs, fakeVersions{}), `{"problem":"sample-sum","language":"python","source":"print(3)"}`)
		if w.Code != http.StatusAccepted {
			t.Fatalf("status = %d, body %s", w.Code, w.Body)
		}
		var got struct{ ID, Status string }
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if !strings.HasPrefix(got.ID, "run-") || got.Status != queue.RunQueued {
			t.Fatalf("response = %+v", got)
		}
		if len(q.jobs) != 1 || q.jobs[0].Kind != queue.KindRun || q.jobs[0].Custom || q.jobs[0].TestSetVersion != fakeVersion {
			t.Fatalf("jobs = %+v", q.jobs)
		}
		if runs.m[got.ID].Status != queue.RunQueued {
			t.Fatalf("run state = %+v", runs.m[got.ID])
		}
	})
	t.Run("custom input travels in the job", func(t *testing.T) {
		q := &fakeQueue{}
		w := postRun(runDeps(q, &fakeRuns{}, fakeVersions{}), `{"problem":"other","language":"go","source":"x","input":""}`)
		if w.Code != http.StatusAccepted || len(q.jobs) != 1 || !q.jobs[0].Custom {
			t.Fatalf("status = %d, jobs = %+v", w.Code, q.jobs)
		}
	})
	tests := []struct {
		name string
		body string
		v    fakeVersions
		want int
	}{
		{"bad json", `nope`, fakeVersions{}, http.StatusBadRequest},
		{"no problem", `{"language":"go","source":"x"}`, fakeVersions{}, http.StatusUnprocessableEntity},
		{"bad language", `{"problem":"sample-sum","language":"cobol","source":"x"}`, fakeVersions{}, http.StatusUnprocessableEntity},
		{"no source", `{"problem":"sample-sum","language":"go","source":""}`, fakeVersions{}, http.StatusUnprocessableEntity},
		{"unknown problem", `{"problem":"nope","language":"go","source":"x"}`, fakeVersions{missing: true}, http.StatusNotFound},
		{"no samples and no input", `{"problem":"other","language":"go","source":"x"}`, fakeVersions{}, http.StatusUnprocessableEntity},
		{"input too large", `{"problem":"sample-sum","language":"go","source":"x","input":"` + strings.Repeat("a", MaxRunInputBytes+1) + `"}`, fakeVersions{}, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeQueue{}
			if w := postRun(runDeps(q, &fakeRuns{}, tc.v), tc.body); w.Code != tc.want || len(q.jobs) != 0 {
				t.Fatalf("status = %d (want %d), jobs = %d", w.Code, tc.want, len(q.jobs))
			}
		})
	}
}

func TestGetRun(t *testing.T) {
	runs := &fakeRuns{m: map[string]queue.RunState{"run-1": {Status: queue.RunDone, Result: &queue.RunResult{Verdict: "OK", Stdout: "3\n"}}}}
	d := runDeps(&fakeQueue{}, runs, fakeVersions{})
	for id, want := range map[string]int{"run-1": http.StatusOK, "run-2": http.StatusNotFound} {
		w := httptest.NewRecorder()
		New(d).ServeHTTP(w, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/runs/"+id, nil))
		if w.Code != want {
			t.Fatalf("GET /runs/%s = %d, want %d", id, w.Code, want)
		}
	}
}
