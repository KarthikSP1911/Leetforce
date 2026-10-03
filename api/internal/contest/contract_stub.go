//go:build leaderboard_stub

// Package contest: STUB of the Phase 14 contract, so Phase 15 compiles on its
// own branch (go build -tags leaderboard_stub ./...). Phase 16 deletes this
// file after merging Phase 14, which provides the real names.
//
// Assumption to reconcile: the real Score may take absolute times or a start
// argument. Phase 15 calls it only through leaderboard/adapter.go, which
// rebases SubmittedAt to "time since contest start, as an offset from the Unix
// epoch", the convention this stub uses.
package contest

import (
	"sort"
	"time"
)

// Event is one submission verdict inside a contest.
type Event struct {
	UserID      string
	ProblemSlug string
	SubmittedAt time.Time
	Verdict     string
}

// Standing is one participant's row.
type Standing struct {
	UserID         string
	Solved         int
	PenaltyMinutes int
	PerProblem     map[string]struct{}
}

// Score applies the ICPC rules: solved desc, penalty asc; penalty is minutes
// to the first AC plus 20 per rejected attempt before it; CE and attempts
// after the first AC are ignored.
func Score(events []Event) []Standing {
	ev := append([]Event(nil), events...)
	sort.SliceStable(ev, func(i, j int) bool { return ev[i].SubmittedAt.Before(ev[j].SubmittedAt) })
	type key struct{ u, p string }
	rejected := map[key]int{}
	done := map[key]bool{}
	byUser := map[string]*Standing{}
	var order []string
	for _, e := range ev {
		s := byUser[e.UserID]
		if s == nil {
			s = &Standing{UserID: e.UserID, PerProblem: map[string]struct{}{}}
			byUser[e.UserID] = s
			order = append(order, e.UserID)
		}
		k := key{e.UserID, e.ProblemSlug}
		switch {
		case done[k] || e.Verdict == "CE" || e.Verdict == "IE":
		case e.Verdict == "AC":
			done[k] = true
			s.Solved++
			s.PenaltyMinutes += int(e.SubmittedAt.Sub(time.Unix(0, 0)).Minutes()) + 20*rejected[k]
		default:
			rejected[k]++
		}
	}
	out := make([]Standing, 0, len(order))
	for _, u := range order {
		out = append(out, *byUser[u])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Solved != out[j].Solved {
			return out[i].Solved > out[j].Solved
		}
		return out[i].PenaltyMinutes < out[j].PenaltyMinutes
	})
	return out
}
