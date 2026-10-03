package leaderboard

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"
)

// Cache tuning. The TTL is only a backstop: a verdict bumps the version, which
// invalidates immediately.
const (
	StandingsTTL = 15 * time.Second
	GlobalTTL    = 60 * time.Second
	DefaultPer   = 50
	MaxPer       = 100
	maxGlobal    = 10000

	globalVerKey = "lb:v:global"
)

func contestVerKey(id string) string { return "lb:v:contest:" + id }

// ErrNoScorer means contests are not available in this build.
var ErrNoScorer = errors.New("leaderboard: contest scoring is not available")

// Service serves standings and the global ranking.
type Service struct {
	src   Source
	cache Cache // nil turns caching off
	score Scorer
	log   *slog.Logger
	now   func() time.Time
}

// New builds a Service. cache and score may be nil.
func New(src Source, cache Cache, score Scorer, log *slog.Logger) *Service {
	return &Service{src: src, cache: cache, score: score, log: log, now: time.Now}
}

// envelope tags a snapshot with the version it was computed under.
type envelope struct {
	V    int64           `json:"v"`
	Data json.RawMessage `json:"data"`
}

// cached returns the snapshot at dataKey if it is still tagged with the
// current version of verKey, else computes and stores a new one. The version
// is read BEFORE the database, so a verdict that commits and bumps while we
// compute leaves our snapshot tagged with an old version, which is never
// served. A cache failure falls back to computing.
func cached[T any](ctx context.Context, s *Service, verKey, dataKey string, ttl time.Duration, compute func(context.Context) (T, error)) (T, error) {
	var zero T
	if s.cache == nil {
		return compute(ctx)
	}
	ver, err := s.cache.KVCounter(ctx, verKey)
	if err != nil {
		s.log.Warn("leaderboard cache version", "err", err)
		return compute(ctx)
	}
	if raw, ok, err := s.cache.KVGet(ctx, dataKey); err == nil && ok {
		var env envelope
		var v T
		if json.Unmarshal(raw, &env) == nil && env.V == ver && json.Unmarshal(env.Data, &v) == nil {
			return v, nil
		}
	}
	v, err := compute(ctx)
	if err != nil {
		return zero, err
	}
	if data, err := json.Marshal(v); err == nil {
		if b, err := json.Marshal(envelope{V: ver, Data: data}); err == nil {
			if err := s.cache.KVSet(ctx, dataKey, b, ttl); err != nil {
				s.log.Warn("leaderboard cache store", "err", err)
			}
		}
	}
	return v, nil
}

// Standings returns the scoreboard of a contest.
func (s *Service) Standings(ctx context.Context, slug string) (ContestStandings, error) {
	if s.score == nil {
		return ContestStandings{}, ErrNoScorer
	}
	c, err := s.src.ContestBySlug(ctx, slug)
	if err != nil {
		return ContestStandings{}, err
	}
	return cached(ctx, s, contestVerKey(c.ID), "lb:contest:"+c.ID, StandingsTTL, func(ctx context.Context) (ContestStandings, error) {
		return s.ComputeStandings(ctx, c)
	})
}

// ComputeStandings builds standings from the database without the cache.
func (s *Service) ComputeStandings(ctx context.Context, c Contest) (ContestStandings, error) {
	if s.score == nil {
		return ContestStandings{}, ErrNoScorer
	}
	events, names, err := s.src.ContestEvents(ctx, c.ID)
	if err != nil {
		return ContestStandings{}, err
	}
	rows := scoreEvents(c, events, names, s.score)
	return ContestStandings{
		Contest:     ContestInfo{Slug: c.Slug, Title: c.Title, StartsAt: c.StartsAt.UTC(), EndsAt: c.EndsAt.UTC()},
		GeneratedAt: s.now().UTC(),
		Problems:    c.Problems,
		Standings:   rows,
	}, nil
}

// Global returns one page of the global ranking.
func (s *Service) Global(ctx context.Context, page, perPage int) (GlobalPage, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = DefaultPer
	}
	perPage = min(perPage, MaxPer)
	all, err := cached(ctx, s, globalVerKey, "lb:global", GlobalTTL, func(ctx context.Context) (GlobalPage, error) {
		rows, err := s.src.GlobalRows(ctx)
		if err != nil {
			return GlobalPage{}, err
		}
		entries := RankGlobal(rows)
		if len(entries) > maxGlobal {
			entries = entries[:maxGlobal]
		}
		return GlobalPage{Total: len(entries), GeneratedAt: s.now().UTC(), Entries: entries}, nil
	})
	if err != nil {
		return GlobalPage{}, err
	}
	lo := min((page-1)*perPage, len(all.Entries))
	hi := min(lo+perPage, len(all.Entries))
	return GlobalPage{Page: page, PerPage: perPage, Total: all.Total, GeneratedAt: all.GeneratedAt, Entries: all.Entries[lo:hi]}, nil
}

// OnVerdict invalidates the snapshots a stored verdict can change. The ingester
// calls it only when RecordVerdict stored or replaced a verdict, so a replayed
// verdict does nothing; and even a redundant bump is harmless, because the next
// read recomputes the same ranking from the database.
func (s *Service) OnVerdict(ctx context.Context, submissionID string) {
	if s.cache == nil {
		return
	}
	if _, err := s.cache.KVIncr(ctx, globalVerKey); err != nil {
		s.log.Warn("leaderboard invalidate global", "err", err)
	}
	cid, ok, err := s.src.SubmissionContest(ctx, submissionID)
	if err != nil {
		s.log.Warn("leaderboard find contest", "submission", submissionID, "err", err)
		return
	}
	if ok {
		if _, err := s.cache.KVIncr(ctx, contestVerKey(cid)); err != nil {
			s.log.Warn("leaderboard invalidate contest", "contest", cid, "err", err)
		}
	}
}
