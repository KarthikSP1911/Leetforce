package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fake mimics the API routes the tester uses.
type fake struct {
	contest      bool // serve /contests/c1/submissions
	limitSubmits int  // first N submits answer 429
	submits      atomic.Int32
	signups      sync.Map
	runPolls     atomic.Int32
}

func (f *fake) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/signup", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]string
		_ = json.NewDecoder(r.Body).Decode(&b)
		if _, dup := f.signups.LoadOrStore(b["username"], true); dup || !strings.HasPrefix(b["username"], "lfload_") {
			http.Error(w, `{"error":"bad"}`, http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("GET /problems", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"problems":[{"slug":"fizz-count"}],"total":1}`))
	})
	mux.HandleFunc("POST /runs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"run-1","status":"queued"}`))
	})
	mux.HandleFunc("GET /runs/{id}", func(w http.ResponseWriter, _ *http.Request) {
		st := "judging"
		if f.runPolls.Add(1)%2 == 0 {
			st = "done"
		}
		_, _ = fmt.Fprintf(w, `{"status":%q}`, st)
	})
	submit := func(w http.ResponseWriter, _ *http.Request) {
		if int(f.submits.Add(1)) <= f.limitSubmits {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"sub-1","status":"queued"}`))
	}
	mux.HandleFunc("POST /submissions", submit)
	if f.contest {
		mux.HandleFunc("POST /contests/c1/submissions", submit)
	}
	mux.HandleFunc("GET /submissions/{id}/events", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		_, _ = fmt.Fprint(w, "event: status\ndata: {\"status\":\"queued\"}\n\n")
		fl.Flush()
		time.Sleep(5 * time.Millisecond)
		_, _ = fmt.Fprint(w, "event: verdict\ndata: {\"status\":\"judged\"}\n\n")
		fl.Flush()
	})
	return mux
}

func stepOf(t *testing.T, r *report, name string) StepReport {
	t.Helper()
	for _, s := range r.Steps {
		if s.Step == name {
			return s
		}
	}
	t.Fatalf("step %q missing in report", name)
	return StepReport{}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		fake       *fake
		cfg        config
		wantErr    error
		wantSubs   int
		wantLimit  int
		wantRunOK  int
		wantSubmit int // successful submit calls, -1 to skip
	}{
		{name: "mixed", fake: &fake{}, cfg: config{Users: 3, Iterations: 2, Mode: "mixed"},
			wantSubs: 6, wantRunOK: 6, wantSubmit: 6},
		{name: "rate limited is not an error", fake: &fake{limitSubmits: 2},
			cfg:      config{Users: 1, Iterations: 3, Mode: "mixed"},
			wantSubs: 1, wantLimit: 2, wantRunOK: 3, wantSubmit: 1},
		{name: "contest mode", fake: &fake{contest: true},
			cfg:      config{Users: 2, Iterations: 1, Mode: "contest", Contest: "c1", Problem: "fizz-count"},
			wantSubs: 2, wantSubmit: 2},
		{name: "contest endpoint missing", fake: &fake{},
			cfg:     config{Users: 2, Iterations: 1, Mode: "contest", Contest: "c1", Problem: "fizz-count"},
			wantErr: errContestMissing, wantSubmit: -1},
		{name: "duration mode", fake: &fake{},
			cfg:      config{Users: 1, Duration: 150 * time.Millisecond, Mode: "contest", Contest: "x", Problem: "p"},
			wantErr:  errContestMissing,
			wantSubs: 0, wantSubmit: -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.fake.handler())
			defer srv.Close()
			tc.cfg.BaseURL = srv.URL
			tc.cfg.PollEvery = time.Millisecond
			tc.cfg.RunID = "t"
			rep, err := run(context.Background(), tc.cfg)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if rep == nil {
				t.Fatal("nil report")
			}
			if tc.wantErr != nil {
				return
			}
			if rep.Submissions != tc.wantSubs || rep.Verdicts != tc.wantSubs {
				t.Errorf("submissions=%d verdicts=%d, want %d", rep.Submissions, rep.Verdicts, tc.wantSubs)
			}
			sub := stepOf(t, rep, "submit")
			if sub.Errors != 0 || sub.RateLimited != tc.wantLimit || sub.OK != tc.wantSubmit {
				t.Errorf("submit step = %+v, want limited=%d ok=%d, no errors", sub, tc.wantLimit, tc.wantSubmit)
			}
			if st := stepOf(t, rep, "stream"); st.Errors != 0 || st.OK != tc.wantSubs {
				t.Errorf("stream step = %+v", st)
			}
			if tc.cfg.Mode == "mixed" {
				if rr := stepOf(t, rep, "run_result"); rr.OK != tc.wantRunOK || rr.Errors != 0 {
					t.Errorf("run_result = %+v, want ok=%d", rr, tc.wantRunOK)
				}
			}
			if sg := stepOf(t, rep, "signup"); sg.OK != tc.cfg.Users {
				t.Errorf("signup ok = %d, want %d", sg.OK, tc.cfg.Users)
			}
			if rep.SubmissionsPerS <= 0 || rep.TimeToVerdict.P50 <= 0 {
				t.Errorf("rates not computed: %+v", rep)
			}
		})
	}
}

func TestNormalizeErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  config
		want string
	}{
		{"no users", config{Mode: "mixed"}, "-users"},
		{"bad mode", config{Users: 1, Mode: "x"}, "unknown -mode"},
		{"contest without slug", config{Users: 1, Mode: "contest"}, "-contest"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.normalize()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestPercentile(t *testing.T) {
	var d []time.Duration
	for i := 1; i <= 100; i++ {
		d = append(d, time.Duration(i)*time.Millisecond)
	}
	tests := []struct {
		p    float64
		want time.Duration
	}{{50, 50 * time.Millisecond}, {95, 95 * time.Millisecond}, {99, 99 * time.Millisecond}, {100, 100 * time.Millisecond}}
	for _, tc := range tests {
		if got := percentile(d, tc.p); got != tc.want {
			t.Errorf("p%v = %v, want %v", tc.p, got, tc.want)
		}
	}
	if percentile(nil, 50) != 0 {
		t.Error("empty slice should give 0")
	}
}

func TestWriteJSON(t *testing.T) {
	srv := httptest.NewServer((&fake{}).handler())
	defer srv.Close()
	rep, err := run(context.Background(), config{BaseURL: srv.URL, Users: 1, Iterations: 1, Mode: "mixed", PollEvery: time.Millisecond, RunID: "j"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "r.json")
	if err := rep.WriteJSON(path); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Clean(path))
	var got report
	if err := json.Unmarshal(b, &got); err != nil || got.Submissions != 1 {
		t.Fatalf("json round trip: %v %+v", err, got)
	}
	var sb strings.Builder
	rep.WriteText(&sb)
	if !strings.Contains(sb.String(), "time to verdict") {
		t.Errorf("text report lacks time to verdict:\n%s", sb.String())
	}
}
