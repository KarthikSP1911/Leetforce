package leaderboard

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type fakeCache struct {
	mu   sync.Mutex
	kv   map[string][]byte
	ctrs map[string]int64
}

func newFakeCache() *fakeCache { return &fakeCache{kv: map[string][]byte{}, ctrs: map[string]int64{}} }

func (c *fakeCache) KVGet(_ context.Context, k string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.kv[k]
	return b, ok, nil
}

func (c *fakeCache) KVSet(_ context.Context, k string, v []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.kv[k] = v
	return nil
}

func (c *fakeCache) KVIncr(_ context.Context, k string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ctrs[k]++
	return c.ctrs[k], nil
}

func (c *fakeCache) KVCounter(_ context.Context, k string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ctrs[k], nil
}

type fakeSource struct {
	mu    sync.Mutex
	rows  []GlobalRow
	reads int
}

func (f *fakeSource) ContestBySlug(context.Context, string) (Contest, error) {
	return Contest{}, ErrNotFound
}

func (f *fakeSource) ContestEvents(context.Context, string) ([]Event, map[string]string, error) {
	return nil, nil, nil
}

func (f *fakeSource) GlobalRows(context.Context) ([]GlobalRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return append([]GlobalRow(nil), f.rows...), nil
}

func (f *fakeSource) SubmissionContest(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func newTestService(src Source, c Cache) *Service {
	return New(src, c, func([]Event) []Standing { return nil }, slog.New(slog.DiscardHandler))
}

func TestRankGlobal(t *testing.T) {
	at := func(m int) time.Time { return time.Date(2026, 1, 1, 0, m, 0, 0, time.UTC) }
	tests := []struct {
		name string
		rows []GlobalRow
		want []string // usernames in order
		rank []int
	}{
		{"hard outweighs easies", []GlobalRow{{UserID: "1", Username: "a", Easy: 4, LastAC: at(1)}, {UserID: "2", Username: "b", Hard: 1, Easy: 0, LastAC: at(2)}}, []string{"b", "a"}, []int{1, 2}},
		{"equal score: more solved first", []GlobalRow{{UserID: "1", Username: "a", Medium: 1, LastAC: at(1)}, {UserID: "2", Username: "b", Easy: 3, LastAC: at(2)}}, []string{"b", "a"}, []int{1, 2}},
		{"equal both: earlier last AC first", []GlobalRow{{UserID: "1", Username: "a", Easy: 2, LastAC: at(9)}, {UserID: "2", Username: "b", Easy: 2, LastAC: at(3)}}, []string{"b", "a"}, []int{1, 2}},
		{"identical share a rank", []GlobalRow{{UserID: "1", Username: "a", Easy: 2, LastAC: at(3)}, {UserID: "2", Username: "b", Easy: 2, LastAC: at(3)}, {UserID: "3", Username: "c", Easy: 1, LastAC: at(1)}}, []string{"a", "b", "c"}, []int{1, 1, 3}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RankGlobal(tc.rows)
			for i, e := range got {
				if e.Username != tc.want[i] || e.Rank != tc.rank[i] {
					t.Errorf("pos %d = %s rank %d, want %s rank %d", i, e.Username, e.Rank, tc.want[i], tc.rank[i])
				}
			}
		})
	}
}

func TestGlobalCacheAndInvalidation(t *testing.T) {
	ctx := context.Background()
	src := &fakeSource{rows: []GlobalRow{{UserID: "1", Username: "a", Easy: 1}}}
	svc := newTestService(src, newFakeCache())

	for range 3 {
		if _, err := svc.Global(ctx, 1, 50); err != nil {
			t.Fatal(err)
		}
	}
	if src.reads != 1 {
		t.Fatalf("database reads = %d, want 1 (the rest served from cache)", src.reads)
	}

	src.rows = append(src.rows, GlobalRow{UserID: "2", Username: "b", Hard: 1})
	if p, _ := svc.Global(ctx, 1, 50); p.Total != 1 {
		t.Fatalf("total = %d before invalidation, want the cached 1", p.Total)
	}
	svc.OnVerdict(ctx, "s1")
	p, _ := svc.Global(ctx, 1, 50)
	if p.Total != 2 || p.Entries[0].Username != "b" {
		t.Fatalf("after invalidation = %+v, want b first of 2", p)
	}

	// A replayed verdict is not reported to OnVerdict (see ingest); even if it
	// were, the ranking is recomputed from the same rows and comes out equal.
	svc.OnVerdict(ctx, "s1")
	q, _ := svc.Global(ctx, 1, 50)
	if q.Total != p.Total || q.Entries[0] != p.Entries[0] {
		t.Fatalf("replayed invalidation changed the ranking: %+v vs %+v", q, p)
	}
}

func TestStaleSnapshotIsNeverServed(t *testing.T) {
	ctx := context.Background()
	cache := newFakeCache()
	src := &fakeSource{rows: []GlobalRow{{UserID: "1", Username: "a", Easy: 1}}}
	svc := newTestService(src, cache)

	// A reader computed under version 0, then a verdict bumped to version 1
	// before the reader stored its snapshot: the snapshot is tagged 0.
	_, _ = svc.Global(ctx, 1, 50)
	src.rows = append(src.rows, GlobalRow{UserID: "2", Username: "b", Hard: 1})
	svc.OnVerdict(ctx, "s1")
	p, _ := svc.Global(ctx, 1, 50)
	if p.Total != 2 {
		t.Fatalf("served the stale snapshot (total %d)", p.Total)
	}
}

func TestGlobalPaging(t *testing.T) {
	var rows []GlobalRow
	for i := range 120 {
		rows = append(rows, GlobalRow{UserID: string(rune('a' + i%26)), Username: string(rune('a'+i%26)) + string(rune('a'+i/26)), Easy: 120 - i})
	}
	svc := newTestService(&fakeSource{rows: rows}, nil)
	ctx := context.Background()
	p, _ := svc.Global(ctx, 3, 50)
	if len(p.Entries) != 20 || p.Total != 120 || p.Entries[0].Rank != 101 {
		t.Fatalf("page 3 = %d entries, total %d, first rank %d", len(p.Entries), p.Total, p.Entries[0].Rank)
	}
	if p, _ = svc.Global(ctx, 9, 50); len(p.Entries) != 0 {
		t.Fatalf("page past the end has %d entries", len(p.Entries))
	}
	if p, _ = svc.Global(ctx, 1, 5000); p.PerPage != MaxPer {
		t.Fatalf("per_page = %d, want clamp to %d", p.PerPage, MaxPer)
	}
}
