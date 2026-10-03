//go:build leaderboard_stub

package leaderboard

import "leetforce/api/internal/contest"

// ContractScorer adapts the Phase 14 contest.Score to Scorer. Phase 16: after
// merging Phase 14, delete the build tag above, the stub in
// api/internal/contest/contract_stub.go and score_none.go.
func ContractScorer() Scorer {
	return func(events []Event) []Standing {
		ev := make([]contest.Event, len(events))
		for i, e := range events {
			ev[i] = contest.Event{UserID: e.UserID, ProblemSlug: e.ProblemSlug, SubmittedAt: e.SubmittedAt, Verdict: e.Verdict}
		}
		st := contest.Score(ev)
		out := make([]Standing, len(st))
		for i, s := range st {
			out[i] = Standing{UserID: s.UserID, Solved: s.Solved, PenaltyMinutes: s.PenaltyMinutes}
		}
		return out
	}
}
