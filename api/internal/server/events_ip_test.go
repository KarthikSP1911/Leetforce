package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"leetforce/api/internal/store"
)

// One client IP cannot take every slot: past its own cap it is refused while
// other IPs still get streams.
func TestStreamSlotsPerIPCap(t *testing.T) {
	s := newStreamSlots(5, 2)
	if !s.acquire("203.0.113.7") || !s.acquire("203.0.113.7") {
		t.Fatal("the first two streams of an IP must be accepted")
	}
	if s.acquire("203.0.113.7") {
		t.Fatal("a third stream from the same IP must be refused")
	}
	if !s.acquire("198.51.100.9") {
		t.Fatal("another IP must still get a slot")
	}
	s.release("203.0.113.7")
	if !s.acquire("203.0.113.7") {
		t.Fatal("a released slot must be reusable by the same IP")
	}
}

func TestStreamPerIPLimitReturns503(t *testing.T) {
	subs := &scriptSubs{steps: []store.Submission{sub("queued")}}
	cfg := EventConfig{PollEvery: 5 * time.Millisecond, Heartbeat: time.Hour, MaxDuration: time.Minute, MaxStreams: 5, MaxStreamsPerIP: 1}
	srv := httptest.NewServer(New(Deps{Submissions: subs, Events: cfg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
	if second.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("second stream from the same IP = %d, want 503", second.StatusCode)
	}
}
