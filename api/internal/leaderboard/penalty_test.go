//go:build leaderboard_stub

package leaderboard

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// ev builds an event `at` after the contest start.
func ev(user, problem string, at time.Duration, verdict string) Event {
	return Event{UserID: user, ProblemSlug: problem, SubmittedAt: t0.Add(at), Verdict: verdict}
}

func TestPenaltyRules(t *testing.T) {
	const m = time.Minute
	contest := Contest{ID: "c", Slug: "c", StartsAt: t0, EndsAt: t0.Add(120 * m)}
	type want struct {
		user            string
		rank, solved, p int
	}
	tests := []struct {
		name   string
		events []Event
		want   []want
	}{
		{"single AC costs its minute", []Event{ev("a", "p1", 10*m, "AC")}, []want{{"a", 1, 1, 10}}},
		{"each rejected attempt before the AC adds 20",
			[]Event{ev("a", "p1", 5*m, "WA"), ev("a", "p1", 8*m, "TLE"), ev("a", "p1", 30*m, "AC")}, []want{{"a", 1, 1, 70}}},
		{"compile errors are ignored", []Event{ev("a", "p1", 2*m, "CE"), ev("a", "p1", 5*m, "AC")}, []want{{"a", 1, 1, 5}}},
		{"attempts after the first AC are ignored",
			[]Event{ev("a", "p1", 10*m, "AC"), ev("a", "p1", 20*m, "WA"), ev("a", "p1", 50*m, "AC")}, []want{{"a", 1, 1, 10}}},
		{"unsolved problem adds no penalty", []Event{ev("a", "p1", 5*m, "WA"), ev("a", "p2", 6*m, "AC")}, []want{{"a", 1, 1, 6}}},
		{"AC exactly at the end time counts", []Event{ev("a", "p1", 120*m, "AC")}, []want{{"a", 1, 1, 120}}},
		{"AC one second after the end is out of the window", []Event{ev("a", "p1", 120*m+time.Second, "AC"), ev("b", "p1", 1*m, "WA")},
			[]want{{"b", 1, 0, 0}}},
		{"submission before the start is ignored", []Event{ev("a", "p1", -time.Second, "AC")}, nil},
		{"more solved beats less penalty",
			[]Event{ev("a", "p1", 1*m, "AC"), ev("b", "p1", 50*m, "AC"), ev("b", "p2", 60*m, "AC")},
			[]want{{"b", 1, 2, 110}, {"a", 2, 1, 1}}},
		{"equal solved: less penalty first",
			[]Event{ev("a", "p1", 30*m, "AC"), ev("b", "p1", 10*m, "AC")}, []want{{"b", 1, 1, 10}, {"a", 2, 1, 30}}},
		{"equal solved and penalty: earlier last AC first",
			[]Event{ev("a", "p1", 30*m, "AC"), ev("a", "p2", 40*m, "AC"), // 70, last AC at 40
				ev("b", "p1", 5*m, "AC"), ev("b", "p2", 25*m+0, "WA"), ev("b", "p2", 45*m, "AC")}, // 5 + 45 + 20 = 70, last AC at 45
			[]want{{"a", 1, 2, 70}, {"b", 2, 2, 70}}},
		{"identical results share a rank",
			[]Event{ev("a", "p1", 10*m, "AC"), ev("b", "p1", 10*m, "AC"), ev("c", "p1", 11*m, "AC")},
			[]want{{"a", 1, 1, 10}, {"b", 1, 1, 10}, {"c", 3, 1, 11}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			names := map[string]string{"a": "a", "b": "b", "c": "c"}
			got := scoreEvents(contest, tc.events, names, ContractScorer())
			if len(got) != len(tc.want) {
				t.Fatalf("got %d rows %+v, want %d", len(got), got, len(tc.want))
			}
			for i, w := range tc.want {
				g := got[i]
				if g.UserID != w.user || g.Rank != w.rank || g.Solved != w.solved || g.PenaltyMinutes != w.p {
					t.Errorf("row %d = {%s rank %d solved %d penalty %d}, want %+v", i, g.UserID, g.Rank, g.Solved, g.PenaltyMinutes, w)
				}
			}
		})
	}
}
