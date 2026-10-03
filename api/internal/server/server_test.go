package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"leetforce/api/internal/metrics"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestHealthAndReady(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		ready    map[string]Pinger
		wantCode int
	}{
		{"healthz needs no dependencies", "/healthz", map[string]Pinger{"database": fakePinger{errors.New("down")}}, 200},
		{"readyz all up", "/readyz", map[string]Pinger{"database": fakePinger{}, "redis": fakePinger{}}, 200},
		{"readyz one down", "/readyz", map[string]Pinger{"database": fakePinger{}, "redis": fakePinger{errors.New("boom")}}, 503},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := New(Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Ready: tc.ready})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequestWithContext(context.Background(), http.MethodGet, tc.path, nil))
			if w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.wantCode, w.Body)
			}
			if tc.wantCode == 503 && (len(w.Body.String()) == 0 || strings.Contains(w.Body.String(), "boom")) {
				t.Fatalf("503 body must report the failing dependency without the error text: %s", w.Body)
			}
		})
	}
}

func TestRequestsAreCountedByRouteTemplate(t *testing.T) {
	r := New(Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	count := func(route, status string) float64 {
		return testutil.ToFloat64(metrics.HTTPRequests.WithLabelValues("GET", route, status))
	}
	okBefore, missBefore := count("/healthz", "200"), count("unmatched", "404")
	for _, path := range []string{"/healthz", "/no/such/path/123"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil))
	}
	if got := count("/healthz", "200") - okBefore; got != 1 {
		t.Errorf("healthz counter moved by %v, want 1", got)
	}
	if got := count("unmatched", "404") - missBefore; got != 1 {
		t.Errorf("unknown path must count under the unmatched route, moved by %v", got)
	}
}
