package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"sort"
	"sync"
	"time"
)

type outcome int

const (
	outcomeOK outcome = iota
	outcomeError
	outcomeRateLimited
)

type stepData struct {
	ok, errs, limited int
	lat               []time.Duration
}

type metrics struct {
	mu       sync.Mutex
	steps    map[string]*stepData
	verdicts []time.Duration
	subs     int
}

func newMetrics() *metrics { return &metrics{steps: map[string]*stepData{}} }

func (m *metrics) record(step string, d time.Duration, o outcome) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.steps[step]
	if s == nil {
		s = &stepData{}
		m.steps[step] = s
	}
	switch o {
	case outcomeOK:
		s.ok++
	case outcomeError:
		s.errs++
	case outcomeRateLimited:
		s.limited++
	}
	if d > 0 {
		s.lat = append(s.lat, d)
	}
}

func (m *metrics) submitted() { m.mu.Lock(); m.subs++; m.mu.Unlock() }

func (m *metrics) verdict(d time.Duration) {
	m.mu.Lock()
	m.verdicts = append(m.verdicts, d)
	m.mu.Unlock()
}

// Stats is a latency summary in milliseconds.
type Stats struct {
	P50 float64 `json:"p50_ms"`
	P95 float64 `json:"p95_ms"`
	P99 float64 `json:"p99_ms"`
	Max float64 `json:"max_ms"`
}

type StepReport struct {
	Step        string `json:"step"`
	OK          int    `json:"ok"`
	Errors      int    `json:"errors"`
	RateLimited int    `json:"rate_limited"`
	Latency     Stats  `json:"latency"`
}

type report struct {
	Mode            string       `json:"mode"`
	BaseURL         string       `json:"base_url"`
	Users           int          `json:"users"`
	ElapsedSec      float64      `json:"elapsed_sec"`
	Steps           []StepReport `json:"steps"`
	Submissions     int          `json:"submissions"`
	Verdicts        int          `json:"verdicts"`
	SubmissionsPerS float64      `json:"submissions_per_sec"`
	TimeToVerdict   Stats        `json:"time_to_verdict"`
}

// percentile uses the nearest-rank method on a sorted slice.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	return sorted[max(rank, 1)-1]
}

func summarize(lat []time.Duration) Stats {
	s := slices.Clone(lat)
	slices.Sort(s)
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	return Stats{P50: ms(percentile(s, 50)), P95: ms(percentile(s, 95)), P99: ms(percentile(s, 99)), Max: ms(percentile(s, 100))}
}

func (m *metrics) report(cfg config, elapsed time.Duration) *report {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := &report{Mode: cfg.Mode, BaseURL: cfg.BaseURL, Users: cfg.Users, ElapsedSec: elapsed.Seconds(),
		Submissions: m.subs, Verdicts: len(m.verdicts), TimeToVerdict: summarize(m.verdicts)}
	if elapsed > 0 {
		r.SubmissionsPerS = float64(m.subs) / elapsed.Seconds()
	}
	names := make([]string, 0, len(m.steps))
	for n := range m.steps {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := m.steps[n]
		r.Steps = append(r.Steps, StepReport{Step: n, OK: s.ok, Errors: s.errs, RateLimited: s.limited, Latency: summarize(s.lat)})
	}
	return r
}

func (r *report) WriteText(w io.Writer) {
	_, _ = fmt.Fprintf(w, "LeetForce load test: mode=%s users=%d base=%s elapsed=%.1fs\n\n", r.Mode, r.Users, r.BaseURL, r.ElapsedSec)
	_, _ = fmt.Fprintf(w, "%-12s %7s %7s %9s %9s %9s %9s\n", "step", "ok", "errors", "429", "p50 ms", "p95 ms", "p99 ms")
	for _, s := range r.Steps {
		_, _ = fmt.Fprintf(w, "%-12s %7d %7d %9d %9.1f %9.1f %9.1f\n", s.Step, s.OK, s.Errors, s.RateLimited, s.Latency.P50, s.Latency.P95, s.Latency.P99)
	}
	_, _ = fmt.Fprintf(w, "\nsubmissions: %d accepted, %d reached a verdict, %.2f/s\n", r.Submissions, r.Verdicts, r.SubmissionsPerS)
	_, _ = fmt.Fprintf(w, "time to verdict: p50 %.0f ms, p95 %.0f ms, p99 %.0f ms, max %.0f ms\n", r.TimeToVerdict.P50, r.TimeToVerdict.P95, r.TimeToVerdict.P99, r.TimeToVerdict.Max)
	_, _ = fmt.Fprintln(w, "note: 429 responses are rate limiting (ADR 0017), counted apart from errors")
}

func (r *report) WriteJSON(path string) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}
