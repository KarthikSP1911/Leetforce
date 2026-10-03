package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Run states. A run is queued by the API, judging once a runner has it, then done.
const (
	RunQueued  = "queued"
	RunJudging = "judging"
	RunDone    = "done"
)

// runTTL is how long a run's state is kept. Runs are throwaway: the browser
// polls for a few seconds and the record is never needed again.
const runTTL = 10 * time.Minute

// RunCase is one sample test in a Run result. Input, Expected, Actual and
// Stderr are set only for a failing sample, which is the one place Run may
// show them; hidden tests never reach a Run result.
type RunCase struct {
	Name     string `json:"name"`
	Verdict  string `json:"verdict"`
	Input    string `json:"input,omitempty"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

// RunResult is the outcome of a Run job. Verdict is a judge verdict for sample
// runs and for custom input one of OK, TLE, MLE, RE, OLE or CE ("OK" means the
// program ran cleanly; there is nothing to compare against).
type RunResult struct {
	Verdict       string    `json:"verdict"`
	RuntimeMS     int64     `json:"runtime_ms"`
	MemoryKB      uint64    `json:"memory_kb"`
	CompileOutput string    `json:"compile_output,omitempty"`
	Cases         []RunCase `json:"cases,omitempty"`
	Stdout        string    `json:"stdout,omitempty"` // custom input only
	Stderr        string    `json:"stderr,omitempty"` // custom input only
}

// RunState is what GET /runs/:id returns.
type RunState struct {
	Status string     `json:"status"`
	Result *RunResult `json:"result,omitempty"`
}

func (q *Queue) runKey(id string) string { return q.cfg.Prefix + ":run:" + id }

// SetRun stores a run's state, replacing any earlier one, and refreshes its TTL.
func (q *Queue) SetRun(ctx context.Context, id string, st RunState) error {
	if id == "" || st.Status == "" {
		return errors.New("set run: id and status are required")
	}
	b, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("set run: %w", err)
	}
	if err := q.rdb.Set(ctx, q.runKey(id), b, runTTL).Err(); err != nil {
		return fmt.Errorf("set run: %w", err)
	}
	return nil
}

// GetRun returns a run's state, and false if it is unknown or expired.
func (q *Queue) GetRun(ctx context.Context, id string) (RunState, bool, error) {
	raw, err := q.rdb.Get(ctx, q.runKey(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return RunState{}, false, nil
	}
	if err != nil {
		return RunState{}, false, fmt.Errorf("get run: %w", err)
	}
	var st RunState
	if err := json.Unmarshal(raw, &st); err != nil {
		return RunState{}, false, fmt.Errorf("decode run %s: %w", id, err)
	}
	return st, true, nil
}
