// Package metrics defines the API's Prometheus metrics (prefix leetforce_) on
// a private registry, so tests and the process share one set and nothing is
// registered twice. Labels are bounded sets (verdicts, languages, routes,
// scopes); never put a user, submission or problem id in a label.
package metrics

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"leetforce/queue"
)

// Registry holds every API metric.
var Registry = prometheus.NewRegistry()

var f = promauto.With(Registry)

// HTTP, submission and verdict metrics, updated where the event happens.
var (
	HTTPRequests = f.NewCounterVec(prometheus.CounterOpts{
		Name: "leetforce_http_requests_total", Help: "HTTP requests by method, route template and status code.",
	}, []string{"method", "route", "status"})
	HTTPSeconds = f.NewHistogramVec(prometheus.HistogramOpts{
		Name: "leetforce_http_request_duration_seconds", Help: "HTTP request latency by method and route template.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"method", "route"})
	SubmissionsCreated = f.NewCounterVec(prometheus.CounterOpts{
		Name: "leetforce_submissions_created_total", Help: "Submissions accepted and stored, by language.",
	}, []string{"language"})
	RunsCreated = f.NewCounterVec(prometheus.CounterOpts{
		Name: "leetforce_runs_created_total", Help: "Run (not Submit) jobs accepted, by language.",
	}, []string{"language"})
	Verdicts = f.NewCounterVec(prometheus.CounterOpts{
		Name: "leetforce_verdicts_total", Help: "Verdicts stored in the database, by verdict (IE is a platform failure).",
	}, []string{"verdict"})
	VerdictWrites = f.NewCounterVec(prometheus.CounterOpts{
		Name: "leetforce_verdict_writes_total", Help: "Ingest outcomes per result: stored, ignored (duplicate or stale), error.",
	}, []string{"outcome"})
	RateLimited = f.NewCounterVec(prometheus.CounterOpts{
		Name: "leetforce_rate_limited_total", Help: "Requests refused with 429, by limit scope (for example submit-user).",
	}, []string{"scope"})
	RateLimitErrors = f.NewCounter(prometheus.CounterOpts{
		Name: "leetforce_rate_limit_errors_total", Help: "Rate-limit checks that failed because Redis was unreachable.",
	})
	StreamsOpen = f.NewGauge(prometheus.GaugeOpts{
		Name: "leetforce_sse_streams_open", Help: "Open SSE status streams.",
	})
	StreamsRefused = f.NewCounter(prometheus.CounterOpts{
		Name: "leetforce_sse_streams_refused_total", Help: "SSE streams refused because the limit was reached.",
	})
	RejudgeSubmissions = f.NewCounter(prometheus.CounterOpts{
		Name: "leetforce_rejudge_submissions_total", Help: "Submissions re-queued because a problem's test set changed.",
	})
	ReaperRequeued = f.NewCounter(prometheus.CounterOpts{
		Name: "leetforce_reaper_requeued_total", Help: "Stored submissions the reaper found never queued and queued again.",
	})
)

// Queue gauges, filled by SampleQueue.
var (
	QueueWaiting = f.NewGauge(prometheus.GaugeOpts{
		Name: "leetforce_queue_waiting", Help: "Jobs in the queue that no runner has taken.",
	})
	QueuePending = f.NewGauge(prometheus.GaugeOpts{
		Name: "leetforce_queue_pending", Help: "Jobs delivered to a runner and not yet acknowledged.",
	})
	QueueOldestSeconds = f.NewGauge(prometheus.GaugeOpts{
		Name: "leetforce_queue_oldest_job_age_seconds", Help: "Age of the oldest unfinished job (0 when the queue is empty).",
	})
	QueueDead = f.NewGauge(prometheus.GaugeOpts{
		Name: "leetforce_queue_dead_letters", Help: "Jobs in the dead-letter stream.",
	})
	QueueSampleOK = f.NewGauge(prometheus.GaugeOpts{
		Name: "leetforce_queue_sample_success", Help: "1 when the last queue sample succeeded, 0 when Redis could not be read.",
	})
	QueueSampleTime = f.NewGauge(prometheus.GaugeOpts{
		Name: "leetforce_queue_sample_timestamp_seconds", Help: "Unix time of the last successful queue sample.",
	})
)

func init() {
	Registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
}

// VerdictLabel keeps the verdict label to the known set so a bad value from a
// runner cannot create new series.
func VerdictLabel(v string) string {
	switch v {
	case "AC", "WA", "TLE", "MLE", "RE", "CE", "OLE", "IE":
		return v
	}
	return "other"
}

// Handler serves the registry in the Prometheus text format.
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{})
}

// QueueStats is the part of *queue.Queue the sampler reads.
type QueueStats interface {
	Stats(ctx context.Context) (queue.Stats, error)
}

// SampleQueue reads the queue depth every interval until ctx ends. Each sample
// costs a few Redis commands, and the hosted Redis plan has a monthly command
// budget, so the interval is a setting (LEETFORCE_METRICS_QUEUE_EVERY).
func SampleQueue(ctx context.Context, q QueueStats, every time.Duration, log *slog.Logger) {
	sample := func() {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		st, err := q.Stats(cctx)
		if err != nil {
			if ctx.Err() == nil {
				QueueSampleOK.Set(0)
				log.Warn("queue sample failed", "err", err)
			}
			return
		}
		QueueWaiting.Set(float64(st.Waiting))
		QueuePending.Set(float64(st.Pending))
		QueueOldestSeconds.Set(st.OldestAge.Seconds())
		QueueDead.Set(float64(st.Dead))
		QueueSampleOK.Set(1)
		QueueSampleTime.SetToCurrentTime()
	}
	sample()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sample()
		}
	}
}
