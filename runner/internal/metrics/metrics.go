// Package metrics defines the runner's Prometheus metrics (prefix
// leetforce_runner_). The runner stays a pure queue consumer: Prometheus pulls
// from this listener, and nothing here talks to the database.
package metrics

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds every runner metric.
var Registry = prometheus.NewRegistry()

var f = promauto.With(Registry)

var (
	Jobs = f.NewCounterVec(prometheus.CounterOpts{
		Name: "leetforce_runner_jobs_total", Help: "Jobs finished by kind (submission or run) and verdict (IE is a platform failure).",
	}, []string{"kind", "verdict"})
	JudgeSeconds = f.NewHistogramVec(prometheus.HistogramOpts{
		Name: "leetforce_runner_judge_duration_seconds", Help: "Wall time to judge one job, by language.",
		Buckets: []float64{.25, .5, 1, 2, 5, 10, 20, 40, 80, 160},
	}, []string{"language"})
	InFlight = f.NewGauge(prometheus.GaugeOpts{
		Name: "leetforce_runner_jobs_in_flight", Help: "Jobs being judged right now.",
	})
	Reclaimed = f.NewCounter(prometheus.CounterOpts{
		Name: "leetforce_runner_reclaimed_jobs_total", Help: "Jobs taken over from another runner that stopped heartbeating.",
	})
	Lost = f.NewCounter(prometheus.CounterOpts{
		Name: "leetforce_runner_lost_jobs_total", Help: "Jobs this runner judged but another runner had taken over, so the result was discarded.",
	})
	HostFailures = f.NewCounterVec(prometheus.CounterOpts{
		Name: "leetforce_runner_host_failures_total", Help: "Failures that are the host's, not the program's, by stage (receive, judge, publish).",
	}, []string{"stage"})
	LastPoll = f.NewGauge(prometheus.GaugeOpts{
		Name: "leetforce_runner_last_poll_timestamp_seconds", Help: "Unix time the runner last got an answer from the queue (a job or an empty poll).",
	})
	Started = f.NewGauge(prometheus.GaugeOpts{
		Name: "leetforce_runner_start_timestamp_seconds", Help: "Unix time the runner started.",
	})
)

// The collectors package would be the current home of these two, but it also
// holds a database/sql stats collector, and a runner must not have database/sql
// anywhere in its dependency graph (runner/nodb_test.go). The constructors in
// the root package are marked obsolete yet behave the same.
func init() {
	Registry.MustRegister(
		prometheus.NewGoCollector(),                                       //nolint:staticcheck // see above
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}), //nolint:staticcheck // see above
	)
}

// VerdictLabel keeps the verdict label to the known set.
func VerdictLabel(v string) string {
	switch v {
	case "AC", "WA", "TLE", "MLE", "RE", "CE", "OLE", "IE":
		return v
	}
	return "other"
}

// LanguageLabel keeps the language label to the supported set.
func LanguageLabel(l string) string {
	switch l {
	case "python", "cpp", "java", "go":
		return l
	}
	return "other"
}

// Handler serves the registry in the Prometheus text format.
func Handler() http.Handler { return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{}) }

// Serve starts /metrics on addr and returns the server so the caller can shut
// it down. An empty addr or "off" disables it and returns nil.
func Serve(addr string, log *slog.Logger) *http.Server {
	if addr == "" || addr == "off" {
		return nil
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", Handler())
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics listener", "addr", addr, "err", err)
		}
	}()
	log.Info("metrics listening", "addr", addr)
	return srv
}
