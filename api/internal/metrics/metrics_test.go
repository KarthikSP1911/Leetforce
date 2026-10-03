package metrics

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"leetforce/queue"
)

type fakeStats struct {
	st  queue.Stats
	err error
}

func (f fakeStats) Stats(context.Context) (queue.Stats, error) { return f.st, f.err }

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSampleQueueSetsGauges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		SampleQueue(ctx, fakeStats{st: queue.Stats{Waiting: 4, Pending: 2, OldestAge: 3 * time.Second, Dead: 1}}, time.Hour, quiet())
		close(done)
	}()
	waitFor(t, func() bool { return testutil.ToFloat64(QueueSampleOK) == 1 })
	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"waiting", testutil.ToFloat64(QueueWaiting), 4},
		{"pending", testutil.ToFloat64(QueuePending), 2},
		{"oldest", testutil.ToFloat64(QueueOldestSeconds), 3},
		{"dead", testutil.ToFloat64(QueueDead), 1},
	} {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	cancel()
	<-done
}

func TestSampleQueueFailureClearsSuccessFlag(t *testing.T) {
	QueueSampleOK.Set(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go SampleQueue(ctx, fakeStats{err: errors.New("redis down")}, time.Hour, quiet())
	waitFor(t, func() bool { return testutil.ToFloat64(QueueSampleOK) == 0 })
	if got := testutil.ToFloat64(QueueSampleOK); got != 0 {
		t.Fatalf("sample_success = %v after a failed sample, want 0", got)
	}
}

func TestHandlerExposesPrefixedMetrics(t *testing.T) {
	HTTPRequests.WithLabelValues("GET", "/healthz", "200").Inc()
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	for _, want := range []string{"leetforce_http_requests_total", "leetforce_queue_waiting", "go_goroutines"} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics output lacks %s", want)
		}
	}
}
