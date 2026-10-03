package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"leetforce/api/internal/store"
	"leetforce/queue"
)

// fakeVersion is the test-set version the fake store stamps on every submission.
const fakeVersion = "ts-0123456789abcdef"

type fakeSubs struct {
	rows     map[string]store.Submission
	noProb   bool
	insertEr error
	deleted  []string
	enqueued []string // ids passed to MarkEnqueued
	markErr  error
	clients  map[string]string // submission id -> client id
	listed   []store.Submission
	listArgs [3]string // problem, client id, limit asked of ListSubmissions
}

func (f *fakeSubs) SetSubmissionClient(_ context.Context, id, clientID string) error {
	if f.clients == nil {
		f.clients = map[string]string{}
	}
	f.clients[id] = clientID
	return nil
}

func (f *fakeSubs) ListSubmissions(_ context.Context, problem, clientID string, limit int) ([]store.Submission, error) {
	f.listArgs = [3]string{problem, clientID, strconv.Itoa(limit)}
	return f.listed, nil
}

func (f *fakeSubs) InsertSubmission(_ context.Context, id, problem, language, _ string) (string, error) {
	if f.insertEr != nil {
		return "", f.insertEr
	}
	if f.noProb {
		return "", store.ErrNotFound
	}
	if f.rows == nil {
		f.rows = map[string]store.Submission{}
	}
	f.rows[id] = store.Submission{ID: id, Problem: problem, Language: language, Status: store.StatusQueued, TestSetVersion: fakeVersion}
	return fakeVersion, nil
}

func (f *fakeSubs) MarkEnqueued(_ context.Context, id string) error {
	f.enqueued = append(f.enqueued, id)
	return f.markErr
}

func (f *fakeSubs) DeleteSubmission(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	delete(f.rows, id)
	return nil
}

func (f *fakeSubs) GetSubmission(_ context.Context, id string) (store.Submission, error) {
	if s, ok := f.rows[id]; ok {
		return s, nil
	}
	return store.Submission{}, store.ErrNotFound
}

type fakeQueue struct {
	jobs []queue.Job
	err  error
}

func (f *fakeQueue) Enqueue(_ context.Context, j queue.Job) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.jobs = append(f.jobs, j)
	return "1-0", nil
}

func post(t *testing.T, d Deps, body string) *httptest.ResponseRecorder {
	t.Helper()
	d.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/submissions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	New(d).ServeHTTP(w, req)
	return w
}

func TestCreateSubmission(t *testing.T) {
	good := `{"problem":"sum","language":"python","source":"print(3)"}`

	t.Run("accepted: row stored and job queued", func(t *testing.T) {
		subs, q := &fakeSubs{}, &fakeQueue{}
		w := post(t, Deps{Submissions: subs, Queue: q}, good)
		var out struct{ ID, Status string }
		if w.Code != http.StatusAccepted || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Status != "queued" || out.ID == "" {
			t.Fatalf("response = %d %s", w.Code, w.Body)
		}
		if len(q.jobs) != 1 || q.jobs[0].SubmissionID != out.ID || q.jobs[0].Source != "print(3)" || q.jobs[0].Problem != "sum" || q.jobs[0].TestSetVersion != fakeVersion {
			t.Fatalf("queued jobs = %+v", q.jobs)
		}
		if _, ok := subs.rows[out.ID]; !ok {
			t.Fatal("submission row not stored")
		}
		if len(subs.enqueued) != 1 || subs.enqueued[0] != out.ID {
			t.Fatalf("MarkEnqueued calls = %v, want [%s] (so the reaper leaves it alone)", subs.enqueued, out.ID)
		}
	})

	t.Run("accepted even if marking it enqueued fails", func(t *testing.T) {
		subs, q := &fakeSubs{markErr: errors.New("database asleep")}, &fakeQueue{}
		w := post(t, Deps{Submissions: subs, Queue: q}, good)
		if w.Code != http.StatusAccepted || len(q.jobs) != 1 || len(subs.deleted) != 0 {
			t.Fatalf("response = %d %s, jobs %d, deleted %v; want 202 with the job queued and the row kept", w.Code, w.Body, len(q.jobs), subs.deleted)
		}
	})

	rejects := []struct {
		name string
		body string
		subs *fakeSubs
		code int
	}{
		{"not JSON", `nope`, &fakeSubs{}, 400},
		{"missing problem", `{"language":"python","source":"x"}`, &fakeSubs{}, 422},
		{"unknown language", `{"problem":"sum","language":"rust","source":"x"}`, &fakeSubs{}, 422},
		{"empty source", `{"problem":"sum","language":"go","source":""}`, &fakeSubs{}, 422},
		{"source over the limit", `{"problem":"sum","language":"go","source":"` + strings.Repeat("a", MaxSourceBytes+1) + `"}`, &fakeSubs{}, 413},
		{"body far over the limit", `{"problem":"sum","language":"go","source":"` + strings.Repeat("a", maxBodyBytes) + `"}`, &fakeSubs{}, 413},
		{"unknown problem", good, &fakeSubs{noProb: true}, 404},
	}
	for _, tc := range rejects {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			q := &fakeQueue{}
			w := post(t, Deps{Submissions: tc.subs, Queue: q}, tc.body)
			if w.Code != tc.code {
				t.Fatalf("status = %d, want %d (%s)", w.Code, tc.code, w.Body)
			}
			if len(q.jobs) != 0 || len(tc.subs.rows) != 0 {
				t.Fatal("a rejected submission must not be stored or queued")
			}
		})
	}

	t.Run("queue failure removes the row and says 503", func(t *testing.T) {
		subs, q := &fakeSubs{}, &fakeQueue{err: errors.New("redis down at secret-host")}
		w := post(t, Deps{Submissions: subs, Queue: q}, good)
		if w.Code != 503 || strings.Contains(w.Body.String(), "secret-host") {
			t.Fatalf("response = %d %s", w.Code, w.Body)
		}
		if len(subs.deleted) != 1 || len(subs.rows) != 0 {
			t.Fatalf("unqueued row not removed: deleted=%v rows=%v", subs.deleted, subs.rows)
		}
	})

	t.Run("database failure is a generic 500", func(t *testing.T) {
		w := post(t, Deps{Submissions: &fakeSubs{insertEr: errors.New("conn refused secret-host")}, Queue: &fakeQueue{}}, good)
		if w.Code != 500 || strings.Contains(w.Body.String(), "secret-host") {
			t.Fatalf("response = %d %s", w.Code, w.Body)
		}
	})
}

func TestGetSubmission(t *testing.T) {
	subs := &fakeSubs{rows: map[string]store.Submission{
		"a": {ID: "a", Problem: "sum", Language: "go", Status: store.StatusJudged, TestSetVersion: "secret-version",
			Verdict: &store.VerdictView{Verdict: "WA", RuntimeMS: 5, MemoryKB: 100, Passed: 2, Total: 5}},
	}}
	d := Deps{Submissions: subs}
	w := get(t, d, "/submissions/a")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"verdict":"WA"`) {
		t.Fatalf("get = %d %s", w.Code, w.Body)
	}
	for _, leak := range []string{"secret-version", "source", "stderr", "expected", "input"} {
		if strings.Contains(w.Body.String(), leak) {
			t.Fatalf("submission response contains %q: %s", leak, w.Body)
		}
	}
	if w := get(t, d, "/submissions/zzz"); w.Code != 404 {
		t.Fatalf("unknown id = %d", w.Code)
	}
}

func TestListSubmissions(t *testing.T) {
	subs := &fakeSubs{listed: []store.Submission{{ID: subID, Problem: "sum", Language: "go", Status: store.StatusJudged,
		Verdict: &store.VerdictView{Verdict: "AC", RuntimeMS: 4, MemoryKB: 900, Passed: 5, Total: 5}}}}
	d := Deps{Submissions: subs}
	list := func(client string) *httptest.ResponseRecorder {
		d.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/problems/sum/submissions", nil)
		if client != "" {
			req.Header.Set(clientHeader, client)
		}
		New(d).ServeHTTP(w, req)
		return w
	}
	t.Run("lists the caller's submissions", func(t *testing.T) {
		w := list("browser-abc12345")
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"AC"`) {
			t.Fatalf("status = %d, body %s", w.Code, w.Body)
		}
		if subs.listArgs != [3]string{"sum", "browser-abc12345", "50"} {
			t.Fatalf("ListSubmissions args = %v", subs.listArgs)
		}
		for _, banned := range []string{"source", "test_set_version", "stderr"} {
			if strings.Contains(w.Body.String(), banned) {
				t.Fatalf("response leaks %q: %s", banned, w.Body)
			}
		}
	})
	for _, bad := range []string{"", "short", "has spaces in it!!"} {
		subs.listArgs = [3]string{}
		w := list(bad)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"submissions":[]`) || subs.listArgs != [3]string{} {
			t.Fatalf("client %q: status = %d, body %s, args %v (want an empty list and no query)", bad, w.Code, w.Body, subs.listArgs)
		}
	}
}

func TestCreateSubmissionTagsClient(t *testing.T) {
	subs, q := &fakeSubs{}, &fakeQueue{}
	d := Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Submissions: subs, Queue: q}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/submissions", strings.NewReader(`{"problem":"sum","language":"go","source":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(clientHeader, "browser-abc12345")
	New(d).ServeHTTP(w, req)
	if w.Code != http.StatusAccepted || len(subs.clients) != 1 {
		t.Fatalf("status = %d, clients = %v", w.Code, subs.clients)
	}
	for _, c := range subs.clients {
		if c != "browser-abc12345" {
			t.Fatalf("client = %q", c)
		}
	}
}
