package leaderboard

import "leetforce/api/internal/contest"

// ContractScorer adapts contest.Score to Scorer. scoreEvents rebases SubmittedAt
// to an offset from epoch, which is exactly contest.Event.Elapsed.
func ContractScorer() Scorer {
	return func(events []Event) []Standing {
		ev := make([]contest.Event, len(events))
		for i, e := range events {
			ev[i] = contest.Event{UserID: e.UserID, ProblemSlug: e.ProblemSlug, SubmittedAt: e.SubmittedAt, Elapsed: e.SubmittedAt.Sub(epoch), Verdict: e.Verdict}
		}
		st := contest.Score(ev)
		out := make([]Standing, len(st))
		for i, s := range st {
			out[i] = Standing{UserID: s.UserID, Solved: s.Solved, PenaltyMinutes: s.PenaltyMinutes}
		}
		return out
	}
}
