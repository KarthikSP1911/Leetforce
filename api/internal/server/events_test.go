package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"leetforce/api/internal/store"
)

const subID = "11111111-1111-4111-8111-111111111111"

// scriptSubs returns the scripted submissions one per call (the last one
// repeats), failing the first `fail` calls after the initial read.
type scriptSubs struct {
	fakeSubs
	mu    sync.Mutex
	steps []store.Submission
	calls int
	fail  int // number of polls (not the first read) that fail
}

func (s *scriptSubs) GetSubmission(_ context.Context, id string) (store.Submission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != subID {
		return store.Submission{}, store.ErrNotFound
	}
	i := s.calls
	s.calls++
	if i > 0 && i <= s.fail {
		return store.Submission{}, errors.New("database asleep at secret-host")
	}
	return s.steps[min(max(i-s.fail, 0), len(s.steps)-1)], nil
}

func sub(status string) store.Submission {
	s := store.Submission{ID: subID, Problem: "sum", Language: "python", Status: status, TestSetVersion: "ts-0123456789abcdef"}
	if status == store.StatusJudged {
		s.Verdict = &store.VerdictView{Verdict: "WA", RuntimeMS: 12, MemoryKB: 3400, Passed: 2, Total: 5}
	}
	return s
}

type sseEvent struct{ Name, Data string }

// parseSSE returns the named events in a stream body (comments and the retry
// hint are not events).
func parseSSE(body string) []sseEvent {
	var out []sseEvent
	for _, block := range strings.Split(body, "\n\n") {
		var ev sseEvent
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.Name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				ev.Data = strings.TrimPrefix(line, "data: ")
			}
		}
		if ev.Name != "" {
			out = append(out, ev)
		}
	}
	return out
}

func fastEvents() EventConfig {
	return EventConfig{PollEvery: 5 * time.Millisecond, Heartbeat: time.Hour, MaxDuration: 5 * time.Second}
}

func stream(t *testing.T, d Deps, id string) *httptest.ResponseRecorder {
	t.Helper()
	d.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/submissions/"+id+"/events", http.NoBody)
	New(d).ServeHTTP(w, req)
	return w
}

func names(evs []sseEvent) []string {
	var n []string
	for _, e := range evs {
		n = append(n, e.Name)
	}
	return n
}

func TestStreamFollowsQueuedJudgingVerdict(t *testing.T) {
	subs := &scriptSubs{steps: []store.Submission{sub("queued"), sub("queued"), sub("judging"), sub("judging"), sub("judged")}}
	w := stream(t, Deps{Submissions: subs, Events: fastEvents()}, subID)

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}
	evs := parseSSE(w.Body.String())
	// One event per change: repeated reads of the same state add nothing.
	if got := strings.Join(names(evs), ","); got != "status,status,verdict" {
		t.Fatalf("events = %s, want status,status,verdict\n%s", got, w.Body)
	}
	if evs[0].Data != `{"status":"queued"}` || evs[1].Data != `{"status":"judging"}` {
		t.Errorf("status payloads = %q, %q", evs[0].Data, evs[1].Data)
	}
	want := `{"status":"judged","verdict":{"verdict":"WA","runtime_ms":12,"memory_kb":3400,"passed":2,"total":5}}`
	if evs[2].Data != want {
		t.Errorf("verdict payload = %s, want %s", evs[2].Data, want)
	}
}

// Submit never leaks: no event may carry the source, the test-set version or
// anything but the state and the verdict view.
func TestStreamCarriesNoHiddenData(t *testing.T) {
	subs := &scriptSubs{steps: []store.Submission{sub("queued"), sub("judged")}}
	w := stream(t, Deps{Submissions: subs, Events: fastEvents()}, subID)
	for _, banned := range []string{"ts-0123456789abcdef", "test_set_version", "source", "stderr", "expected", "input"} {
		if strings.Contains(w.Body.String(), banned) {
			t.Errorf("stream contains %q:\n%s", banned, w.Body)
		}
	}
}

func TestStreamLateClientGetsVerdictAtOnce(t *testing.T) {
	subs := &scriptSubs{steps: []store.Submission{sub("judged")}}
	w := stream(t, Deps{Submissions: subs, Events: fastEvents()}, subID)
	if got := strings.Join(names(parseSSE(w.Body.String())), ","); got != "verdict" {
		t.Fatalf("events = %s, want only verdict", got)
	}
	if subs.calls != 1 {
		t.Errorf("store read %d times, want 1 (no polling once judged)", subs.calls)
	}
}

func TestStreamUnknownSubmission(t *testing.T) {
	w := stream(t, Deps{Submissions: &scriptSubs{steps: []store.Submission{sub("queued")}}, Events: fastEvents()}, "22222222-2222-4222-8222-222222222222")
	if w.Code != http.StatusNotFound || strings.Contains(w.Header().Get("Content-Type"), "event-stream") {
		t.Fatalf("response = %d %s (%s)", w.Code, w.Body, w.Header().Get("Content-Type"))
	}
}

func TestStreamToleratesTransientReadErrors(t *testing.T) {
	subs := &scriptSubs{fail: 3, steps: []store.Submission{sub("queued"), sub("judged")}}
	cfg := fastEvents()
	cfg.MaxErrors = 10
	w := stream(t, Deps{Submissions: subs, Events: cfg}, subID)
	if got := strings.Join(names(parseSSE(w.Body.String())), ","); got != "status,verdict" {
		t.Fatalf("events = %s, want status,verdict after 3 failed polls", got)
	}
}

func TestStreamGivesUpAfterRepeatedErrorsWithoutLeakingThem(t *testing.T) {
	subs := &scriptSubs{fail: 100, steps: []store.Submission{sub("queued")}}
	cfg := fastEvents()
	cfg.MaxErrors = 3
	w := stream(t, Deps{Submissions: subs, Events: cfg}, subID)
	if got := strings.Join(names(parseSSE(w.Body.String())), ","); got != "status,error" {
		t.Fatalf("events = %s, want status,error", got)
	}
	if strings.Contains(w.Body.String(), "secret-host") {
		t.Fatalf("the stream leaked an internal error:\n%s", w.Body)
	}
}

func TestStreamEndsAtTimeLimit(t *testing.T) {
	subs := &scriptSubs{steps: []store.Submission{sub("queued")}}
	cfg := fastEvents()
	cfg.MaxDuration = 50 * time.Millisecond
	w := stream(t, Deps{Submissions: subs, Events: cfg}, subID)
	if got := strings.Join(names(parseSSE(w.Body.String())), ","); got != "status,timeout" {
		t.Fatalf("events = %s, want status,timeout", got)
	}
}

func TestStreamSendsHeartbeats(t *testing.T) {
	subs := &scriptSubs{steps: []store.Submission{sub("queued")}}
	cfg := fastEvents()
	cfg.Heartbeat = 10 * time.Millisecond
	cfg.MaxDuration = 100 * time.Millisecond
	w := stream(t, Deps{Submissions: subs, Events: cfg}, subID)
	if !strings.Contains(w.Body.String(), ": keep-alive\n\n") {
		t.Fatalf("no keep-alive comment in:\n%s", w.Body)
	}
}

// Streams are capped per instance, and a client that goes away frees its slot.
func TestStreamLimitAndRelease(t *testing.T) {
	subs := &scriptSubs{steps: []store.Submission{sub("queued")}}
	cfg := EventConfig{PollEvery: 5 * time.Millisecond, Heartbeat: time.Hour, MaxDuration: time.Minute, MaxStreams: 1}
	srv := httptest.NewServer(New(Deps{Submissions: subs, Events: cfg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/submissions/"+subID+"/events", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	first, err := http.DefaultClient.Do(req)
	if err != nil || first.StatusCode != http.StatusOK {
		t.Fatalf("first stream: %v %v", first, err)
	}
	defer func() { _ = first.Body.Close() }()

	second, err := http.Get(srv.URL + "/submissions/" + subID + "/events") //nolint:gosec,noctx // test server URL
	if err != nil {
		t.Fatal(err)
	}
	_ = second.Body.Close()
	if second.StatusCode != http.StatusServiceUnavailable || second.Header.Get("Retry-After") == "" {
		t.Fatalf("second stream = %d (Retry-After %q), want 503 with Retry-After", second.StatusCode, second.Header.Get("Retry-After"))
	}

	cancel() // the first client disconnects
	deadline := time.Now().Add(2 * time.Second)
	for {
		r, err := http.Get(srv.URL + "/submissions/" + subID + "/events") //nolint:gosec,noctx // test server URL
		if err != nil {
			t.Fatal(err)
		}
		_ = r.Body.Close()
		if r.StatusCode == http.StatusOK {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("slot was not released after the client disconnected (last status %d)", r.StatusCode)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
