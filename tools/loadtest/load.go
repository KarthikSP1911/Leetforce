package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type config struct {
	BaseURL        string
	Users          int
	Iterations     int
	Duration       time.Duration
	Ramp           time.Duration
	Mode           string
	Contest        string
	Problem        string
	Language       string
	Source         string
	SourceFile     string
	Password       string
	VerdictTimeout time.Duration
	JSONFile       string
	// RunID names the accounts; empty means a fresh time-based id.
	RunID string
	// PollEvery is the Run result poll interval (default 300ms).
	PollEvery time.Duration
	// MaxRetryWait caps how long a 429's Retry-After is honored (default 5s).
	MaxRetryWait time.Duration
}

const defaultSource = "print(0)\n"

// errContestMissing aborts the whole run when the contest cannot be found.
var errContestMissing = errors.New("contest endpoint answered 404 (contest API not deployed or wrong -contest slug)")

func (c *config) normalize() error {
	if c.Users < 1 {
		return errors.New("-users must be at least 1")
	}
	if c.Iterations == 0 && c.Duration == 0 {
		c.Iterations = 1
	}
	switch c.Mode {
	case "mixed":
	case "contest":
		if c.Contest == "" {
			return errors.New("-mode contest needs -contest <slug>")
		}
	default:
		return fmt.Errorf("unknown -mode %q (mixed or contest)", c.Mode)
	}
	if c.Source == "" {
		c.Source = defaultSource
	}
	if c.RunID == "" {
		c.RunID = strconv.FormatInt(time.Now().Unix(), 36)
	}
	if c.PollEvery <= 0 {
		c.PollEvery = 300 * time.Millisecond
	}
	if c.MaxRetryWait <= 0 {
		c.MaxRetryWait = 5 * time.Second
	}
	if c.VerdictTimeout <= 0 {
		c.VerdictTimeout = 2 * time.Minute
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	return nil
}

// run executes the load test and returns the report. A non-nil error with a
// non-nil report means the run was aborted early.
func run(ctx context.Context, cfg config) (*report, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	m := newMetrics()
	start := time.Now()
	var deadline time.Time
	if cfg.Duration > 0 {
		deadline = start.Add(cfg.Duration)
	}
	var wg sync.WaitGroup
	for i := 0; i < cfg.Users; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if cfg.Ramp > 0 && cfg.Users > 1 {
				delay := cfg.Ramp * time.Duration(n) / time.Duration(cfg.Users-1)
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return
				}
			}
			newVU(cfg, n, m, cancel).loop(ctx, deadline)
		}(i)
	}
	wg.Wait()
	rep := m.report(cfg, time.Since(start))
	if cause := context.Cause(ctx); errors.Is(cause, errContestMissing) {
		return rep, cause
	}
	return rep, nil
}

type vu struct {
	cfg    config
	n      int
	m      *metrics
	client *http.Client
	abort  context.CancelCauseFunc
}

func newVU(cfg config, n int, m *metrics, abort context.CancelCauseFunc) *vu {
	jar, _ := cookiejar.New(nil)
	return &vu{cfg: cfg, n: n, m: m, abort: abort, client: &http.Client{Jar: jar}}
}

func (v *vu) loop(ctx context.Context, deadline time.Time) {
	name := fmt.Sprintf("lfload_%s_%d", v.cfg.RunID, v.n)
	if !v.signup(ctx, name) {
		return
	}
	slug := v.cfg.Problem
	if v.cfg.Mode == "contest" {
		if !v.register(ctx) {
			return
		}
		if slug == "" {
			slug = v.contestProblem(ctx)
		}
	}
	for it := 0; ; it++ {
		if ctx.Err() != nil {
			return
		}
		if v.cfg.Iterations > 0 && it >= v.cfg.Iterations {
			return
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return
		}
		if v.cfg.Mode == "mixed" {
			if s := v.problems(ctx); slug == "" {
				slug = s
			}
		}
		if slug == "" {
			// Nothing to submit to; do not spin.
			return
		}
		if v.cfg.Mode == "mixed" {
			v.run(ctx, slug)
		}
		v.submit(ctx, slug)
	}
}

// do sends one request and records it under step. A 429 is counted apart from
// errors and waited out; ok is true only for the wanted status.
func (v *vu) do(ctx context.Context, step, method, path string, body any, want int) (resp *http.Response, data []byte, ok bool) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, v.cfg.BaseURL+path, rd)
	if err != nil {
		v.m.record(step, 0, outcomeError)
		return nil, nil, false
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	t0 := time.Now()
	resp, err = v.client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			v.m.record(step, time.Since(t0), outcomeError)
		}
		return nil, nil, false
	}
	data, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	d := time.Since(t0)
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		v.m.record(step, d, outcomeRateLimited)
		wait := time.Second
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
			wait = time.Duration(s) * time.Second
		}
		wait = min(wait, v.cfg.MaxRetryWait)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
		}
		return resp, data, false
	case want:
		v.m.record(step, d, outcomeOK)
		return resp, data, true
	default:
		v.m.record(step, d, outcomeError)
		return resp, data, false
	}
}

func (v *vu) signup(ctx context.Context, name string) bool {
	body := map[string]string{"email": name + "@loadtest.invalid", "username": name, "password": v.cfg.Password}
	for range 5 {
		if _, _, ok := v.do(ctx, "signup", http.MethodPost, "/auth/signup", body, http.StatusCreated); ok {
			return true
		}
		if ctx.Err() != nil {
			return false
		}
	}
	return false
}

// register joins the contest; a 404 means the contest does not exist, which
// aborts the whole run.
func (v *vu) register(ctx context.Context) bool {
	path := "/contests/" + url.PathEscape(v.cfg.Contest) + "/register"
	for range 5 {
		resp, _, ok := v.do(ctx, "register", http.MethodPost, path, struct{}{}, http.StatusOK)
		if ok {
			return true
		}
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			v.abort(errContestMissing)
			return false
		}
		if ctx.Err() != nil {
			return false
		}
	}
	return false
}

// contestProblem returns the first problem of the contest.
func (v *vu) contestProblem(ctx context.Context) string {
	_, data, ok := v.do(ctx, "contest_problems", http.MethodGet, "/contests/"+url.PathEscape(v.cfg.Contest)+"/problems", nil, http.StatusOK)
	if !ok {
		return ""
	}
	var body struct {
		Problems []struct {
			Slug string `json:"slug"`
		} `json:"problems"`
	}
	if json.Unmarshal(data, &body) != nil || len(body.Problems) == 0 {
		v.m.record("contest_problems", 0, outcomeError)
		return ""
	}
	return body.Problems[0].Slug
}

// problems lists problems and returns the first slug.
func (v *vu) problems(ctx context.Context) string {
	_, data, ok := v.do(ctx, "problems", http.MethodGet, "/problems", nil, http.StatusOK)
	if !ok {
		return ""
	}
	var page struct {
		Problems []struct {
			Slug string `json:"slug"`
		} `json:"problems"`
	}
	if json.Unmarshal(data, &page) != nil || len(page.Problems) == 0 {
		v.m.record("problems", 0, outcomeError)
		return ""
	}
	return page.Problems[0].Slug
}

type idResp struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// run queues a Run on the samples and polls until it is done.
func (v *vu) run(ctx context.Context, slug string) {
	body := map[string]any{"problem": slug, "language": v.cfg.Language, "source": v.cfg.Source}
	_, data, ok := v.do(ctx, "run", http.MethodPost, "/runs", body, http.StatusAccepted)
	if !ok {
		return
	}
	var r idResp
	if json.Unmarshal(data, &r) != nil || r.ID == "" {
		v.m.record("run", 0, outcomeError)
		return
	}
	t0 := time.Now()
	for time.Since(t0) < v.cfg.VerdictTimeout {
		select {
		case <-time.After(v.cfg.PollEvery):
		case <-ctx.Done():
			return
		}
		_, data, ok := v.do(ctx, "run_poll", http.MethodGet, "/runs/"+r.ID, nil, http.StatusOK)
		if !ok {
			continue
		}
		var st idResp
		if json.Unmarshal(data, &st) == nil && st.Status == "done" {
			v.m.record("run_result", time.Since(t0), outcomeOK)
			return
		}
	}
	v.m.record("run_result", time.Since(t0), outcomeError)
}

func (v *vu) submit(ctx context.Context, slug string) {
	path := "/submissions"
	body := map[string]any{"problem": slug, "language": v.cfg.Language, "source": v.cfg.Source}
	if v.cfg.Mode == "contest" {
		body["contest_id"] = v.cfg.Contest
	}
	t0 := time.Now()
	resp, data, ok := v.do(ctx, "submit", http.MethodPost, path, body, http.StatusAccepted)
	if resp != nil && resp.StatusCode == http.StatusNotFound && v.cfg.Mode == "contest" {
		v.abort(errContestMissing)
		return
	}
	if !ok {
		return
	}
	var r idResp
	if json.Unmarshal(data, &r) != nil || r.ID == "" {
		v.m.record("submit", 0, outcomeError)
		return
	}
	v.m.submitted()
	v.stream(ctx, r.ID, t0)
}

// stream follows the SSE status stream until a verdict event arrives.
func (v *vu) stream(ctx context.Context, id string, submitStart time.Time) {
	sctx, cancel := context.WithTimeout(ctx, v.cfg.VerdictTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(sctx, http.MethodGet, v.cfg.BaseURL+"/submissions/"+id+"/events", nil)
	if err != nil {
		v.m.record("stream", 0, outcomeError)
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	t0 := time.Now()
	resp, err := v.client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			v.m.record("stream", time.Since(t0), outcomeError)
		}
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		// 503 is the stream-slot cap; both are back-pressure, not failures.
		v.m.record("stream", time.Since(t0), outcomeRateLimited)
		return
	}
	if resp.StatusCode != http.StatusOK {
		v.m.record("stream", time.Since(t0), outcomeError)
		return
	}
	sc := bufio.NewScanner(resp.Body)
	event := ""
	for sc.Scan() {
		line := sc.Text()
		if after, found := strings.CutPrefix(line, "event:"); found {
			event = strings.TrimSpace(after)
		}
		if line == "" && event == "verdict" {
			v.m.record("stream", time.Since(t0), outcomeOK)
			v.m.verdict(time.Since(submitStart))
			return
		}
		if line == "" {
			event = ""
		}
	}
	if ctx.Err() == nil {
		v.m.record("stream", time.Since(t0), outcomeError)
	}
}
