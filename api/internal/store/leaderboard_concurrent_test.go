//go:build leaderboard_concurrent

// Phase 15 exit test: rankings stay correct under concurrent verdict ingests.
// Run with scripts/test-leaderboard-concurrent.sh (make test-leaderboard-concurrent):
// it needs DATABASE_URL, the Phase 14 migrations (contests, contest_problems,
// contest_participants, submissions.contest_id) and the race detector.
// The tables are created in a throwaway schema by testStore.
package store

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"leetforce/api/internal/leaderboard"
)

// memCache is a Redis stand-in with the same semantics the Service relies on.
type memCache struct {
	mu   sync.Mutex
	kv   map[string][]byte
	ctrs map[string]int64
}

func newMemCache() *memCache { return &memCache{kv: map[string][]byte{}, ctrs: map[string]int64{}} }

func (m *memCache) KVGet(_ context.Context, k string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.kv[k]
	return b, ok, nil
}

func (m *memCache) KVSet(_ context.Context, k string, v []byte, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.kv[k] = v
	return nil
}

func (m *memCache) KVIncr(_ context.Context, k string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctrs[k]++
	return m.ctrs[k], nil
}

func (m *memCache) KVCounter(_ context.Context, k string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ctrs[k], nil
}

func TestLeaderboardConcurrentIngest(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	const (
		nUsers   = 8
		nSubs    = 240
		nWorkers = 32
	)
	problems := []struct{ slug, diff string }{{"lb-easy", "easy"}, {"lb-medium", "medium"}, {"lb-hard", "hard"}}
	for _, p := range problems {
		if _, err := s.pool.Exec(ctx, `INSERT INTO problems (slug, title, difficulty, test_set_version) VALUES ($1, $1, $2, 'v1')`, p.slug, p.diff); err != nil {
			t.Fatal(err)
		}
	}
	users := make([]string, nUsers)
	for i := range users {
		users[i] = uuid.NewString()
		if _, err := s.pool.Exec(ctx, `INSERT INTO users (id, email, username, password_hash) VALUES ($1, $2, $3, 'x')`,
			users[i], fmt.Sprintf("u%d@example.test", i), fmt.Sprintf("user%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	contestID := uuid.NewString()
	if _, err := s.pool.Exec(ctx, `INSERT INTO contests (id, slug, title, starts_at, ends_at) VALUES ($1, 'lb-test', 'LB test', $2, $3)`,
		contestID, start, start.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for i, p := range problems {
		if _, err := s.pool.Exec(ctx, `INSERT INTO contest_problems (contest_id, problem_slug, label, position) VALUES ($1, $2, $3, $4)`, contestID, p.slug, string(rune('A'+i)), i); err != nil {
			t.Fatal(err)
		}
	}
	for _, u := range users {
		if _, err := s.pool.Exec(ctx, `INSERT INTO contest_participants (contest_id, user_id) VALUES ($1, $2)`, contestID, u); err != nil {
			t.Fatal(err)
		}
	}
	rng := rand.New(rand.NewPCG(1, 2))
	subIDs := make([]string, nSubs)
	for i := range subIDs {
		subIDs[i] = uuid.NewString()
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO submissions (id, problem_slug, language, source, test_set_version, user_id, contest_id, created_at)
			VALUES ($1, $2, 'python', '', 'v1', $3, $4, $5)`,
			subIDs[i], problems[rng.IntN(len(problems))].slug, users[rng.IntN(nUsers)], contestID,
			start.Add(time.Duration(i)*time.Minute+time.Duration(rng.IntN(60))*time.Second)); err != nil {
			t.Fatal(err)
		}
	}

	svc := leaderboard.New(s, newMemCache(), leaderboard.ContractScorer(), slog.New(slog.DiscardHandler))
	verdicts := []string{"AC", "WA", "WA", "TLE", "CE", "RE"}

	// Every verdict is delivered twice (a replay) by different workers, while
	// readers hammer both rankings through the cache.
	jobs := make(chan string, nSubs*2)
	for _, id := range subIDs {
		jobs <- id
		jobs <- id
	}
	close(jobs)
	verdictOf := make(map[string]string, nSubs)
	for _, id := range subIDs {
		verdictOf[id] = verdicts[rng.IntN(len(verdicts))]
	}

	stop := make(chan struct{})
	var readers, writers sync.WaitGroup
	for r := 0; r < 4; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := svc.Standings(ctx, "lb-test"); err != nil {
					t.Errorf("standings during ingest: %v", err)
					return
				}
				if _, err := svc.Global(ctx, 1, 50); err != nil {
					t.Errorf("global during ingest: %v", err)
					return
				}
			}
		}()
	}
	for w := 0; w < nWorkers; w++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for id := range jobs {
				rec, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: id, Verdict: verdictOf[id], TestSetVersion: "v1", RunnerID: "t"})
				if err != nil {
					t.Errorf("record verdict: %v", err)
					return
				}
				if rec { // same rule as the ingester: only a stored verdict invalidates
					svc.OnVerdict(ctx, id)
				}
			}
		}()
	}
	writers.Wait()
	close(stop)
	readers.Wait()

	c, err := s.ContestBySlug(ctx, "lb-test")
	if err != nil {
		t.Fatal(err)
	}
	want, err := svc.ComputeStandings(ctx, c) // straight from the database, no cache
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Standings(ctx, "lb-test") // through the cache
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Standings, want.Standings) {
		t.Fatalf("cached standings differ from the database after the last verdict\n got: %+v\nwant: %+v", got.Standings, want.Standings)
	}

	// Independent oracle: Score over the events stored in the database.
	events, _, err := s.ContestEvents(ctx, contestID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != nSubs {
		t.Fatalf("events in database = %d, want %d (a verdict was lost or doubled)", len(events), nSubs)
	}
	// The oracle applies the same window rule as the service: a verdict after
	// the contest end does not count (the test stores submissions past it).
	rebased := make([]leaderboard.Event, 0, len(events))
	for _, e := range events {
		if e.SubmittedAt.After(c.EndsAt) {
			continue
		}
		e.SubmittedAt = time.Unix(0, 0).UTC().Add(e.SubmittedAt.Sub(c.StartsAt))
		rebased = append(rebased, e)
	}
	byUser := map[string]leaderboard.Standing{}
	for _, st := range leaderboard.ContractScorer()(rebased) {
		byUser[st.UserID] = st
	}
	for _, row := range want.Standings {
		o := byUser[row.UserID]
		if row.Solved != o.Solved || row.PenaltyMinutes != o.PenaltyMinutes {
			t.Errorf("user %s: standings %d solved / %d min, Score over DB events %d / %d",
				row.Username, row.Solved, row.PenaltyMinutes, o.Solved, o.PenaltyMinutes)
		}
	}
	for i := 1; i < len(got.Standings); i++ {
		a, b := got.Standings[i-1], got.Standings[i]
		if a.Solved < b.Solved || (a.Solved == b.Solved && a.PenaltyMinutes > b.PenaltyMinutes) {
			t.Errorf("standings out of order at %d: %+v before %+v", i, a, b)
		}
	}

	// The global ranking must equal a direct recount of AC verdicts.
	g, err := svc.Global(ctx, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := s.GlobalRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if wantG := leaderboard.RankGlobal(direct); !reflect.DeepEqual(g.Entries, wantG) {
		t.Fatalf("cached global ranking differs from the database\n got: %+v\nwant: %+v", g.Entries, wantG)
	}
}
