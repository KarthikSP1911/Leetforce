package contest

import (
	"slices"
	"sort"
	"time"
)

// RejectedPenaltyMinutes is added for each rejected attempt before the first AC.
const RejectedPenaltyMinutes = 20

// Event is one judged contest submission.
//
// Elapsed is the time from contest start to SubmittedAt. The store fills it
// (and drops attempts outside the window) because Score has no contest to
// measure against; see docs/phases/phase-14-contract.md.
type Event struct {
	UserID      string
	ProblemSlug string
	SubmittedAt time.Time
	Elapsed     time.Duration
	Verdict     string
}

// ProblemResult is one user's result on one problem.
type ProblemResult struct {
	ProblemSlug string `json:"problem"`
	Solved      bool   `json:"solved"`
	// Rejected counts attempts that failed before the first AC (all of them if unsolved).
	Rejected int `json:"rejected"`
	// SolvedAtMinutes is whole minutes from contest start to the first AC.
	SolvedAtMinutes int `json:"solved_at_minutes"`
	// PenaltyMinutes is SolvedAtMinutes + 20 per rejected attempt; 0 if unsolved.
	PenaltyMinutes int `json:"penalty_minutes"`
}

// Standing is one user's row of the standings.
type Standing struct {
	UserID         string          `json:"user_id"`
	Rank           int             `json:"rank"`
	Solved         int             `json:"solved"`
	PenaltyMinutes int             `json:"penalty_minutes"`
	PerProblem     []ProblemResult `json:"per_problem"`
}

// counts reports whether a verdict decides an attempt. CE is not counted (the
// code never ran) and IE is an internal error, not the contestant's fault.
// Empty means not judged yet.
func counts(verdict string) bool {
	switch verdict {
	case "", "CE", "IE":
		return false
	}
	return true
}

// Score ranks users ICPC style: more problems solved first, then less total
// penalty. A solved problem costs the whole minutes from contest start to its
// first AC plus 20 minutes per rejected attempt before it. Attempts after the
// first AC, CE/IE verdicts and attempts before the start (negative Elapsed) do
// not count. Users with equal solved and penalty share a rank (1, 1, 3).
// Users with no counted event do not appear.
func Score(events []Event) []Standing {
	evs := slices.Clone(events)
	sort.SliceStable(evs, func(i, j int) bool {
		if !evs[i].SubmittedAt.Equal(evs[j].SubmittedAt) {
			return evs[i].SubmittedAt.Before(evs[j].SubmittedAt)
		}
		return evs[i].Elapsed < evs[j].Elapsed
	})

	type key struct{ user, problem string }
	results := map[key]*ProblemResult{}
	order := map[string][]string{} // user -> problems seen
	for _, e := range evs {
		if e.Elapsed < 0 || !counts(e.Verdict) {
			continue
		}
		k := key{e.UserID, e.ProblemSlug}
		r, ok := results[k]
		if !ok {
			r = &ProblemResult{ProblemSlug: e.ProblemSlug}
			results[k] = r
			order[e.UserID] = append(order[e.UserID], e.ProblemSlug)
		}
		if r.Solved {
			continue
		}
		if e.Verdict == "AC" {
			r.Solved = true
			r.SolvedAtMinutes = int(e.Elapsed / time.Minute)
			r.PenaltyMinutes = r.SolvedAtMinutes + RejectedPenaltyMinutes*r.Rejected
			continue
		}
		r.Rejected++
	}

	out := make([]Standing, 0, len(order))
	for user, problems := range order {
		st := Standing{UserID: user, PerProblem: []ProblemResult{}}
		slices.Sort(problems)
		for _, p := range problems {
			r := *results[key{user, p}]
			st.PerProblem = append(st.PerProblem, r)
			if r.Solved {
				st.Solved++
				st.PenaltyMinutes += r.PenaltyMinutes
			}
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Solved != b.Solved {
			return a.Solved > b.Solved
		}
		if a.PenaltyMinutes != b.PenaltyMinutes {
			return a.PenaltyMinutes < b.PenaltyMinutes
		}
		return a.UserID < b.UserID
	})
	for i := range out {
		if i > 0 && out[i].Solved == out[i-1].Solved && out[i].PenaltyMinutes == out[i-1].PenaltyMinutes {
			out[i].Rank = out[i-1].Rank
		} else {
			out[i].Rank = i + 1
		}
	}
	return out
}
