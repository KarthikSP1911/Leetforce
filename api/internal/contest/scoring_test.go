package contest

import (
	"reflect"
	"testing"
	"time"
)

func ev(user, problem string, min float64, verdict string) Event {
	d := time.Duration(min * float64(time.Minute))
	return Event{UserID: user, ProblemSlug: problem, SubmittedAt: time.Unix(0, 0).Add(d), Elapsed: d, Verdict: verdict}
}

type row struct {
	User    string
	Rank    int
	Solved  int
	Penalty int
}

func rows(s []Standing) []row {
	out := make([]row, len(s))
	for i, st := range s {
		out[i] = row{st.UserID, st.Rank, st.Solved, st.PenaltyMinutes}
	}
	return out
}

func TestScore(t *testing.T) {
	tests := []struct {
		name   string
		events []Event
		want   []row
	}{
		{"none", nil, []row{}},
		{"single AC at 10m59s", []Event{ev("a", "p1", 10.99, "AC")}, []row{{"a", 1, 1, 10}}},
		{"rejected before AC cost 20 each", []Event{
			ev("a", "p1", 5, "WA"), ev("a", "p1", 8, "TLE"), ev("a", "p1", 30, "AC"),
		}, []row{{"a", 1, 1, 70}}},
		{"CE does not count", []Event{
			ev("a", "p1", 1, "CE"), ev("a", "p1", 12, "AC"),
		}, []row{{"a", 1, 1, 12}}},
		{"IE does not count", []Event{ev("a", "p1", 1, "IE"), ev("a", "p1", 12, "AC")}, []row{{"a", 1, 1, 12}}},
		{"attempts after first AC ignored", []Event{
			ev("a", "p1", 10, "AC"), ev("a", "p1", 20, "WA"), ev("a", "p1", 30, "AC"),
		}, []row{{"a", 1, 1, 10}}},
		{"unsolved adds no penalty", []Event{
			ev("a", "p1", 5, "WA"), ev("a", "p2", 6, "AC"),
		}, []row{{"a", 1, 1, 6}}},
		{"only CE: user absent", []Event{ev("a", "p1", 5, "CE")}, []row{}},
		{"before start ignored", []Event{ev("a", "p1", -1, "AC")}, []row{}},
		{"more solved beats less penalty", []Event{
			ev("a", "p1", 100, "AC"), ev("a", "p2", 100, "AC"),
			ev("b", "p1", 1, "AC"),
		}, []row{{"a", 1, 2, 200}, {"b", 2, 1, 1}}},
		{"equal solved: lower penalty wins", []Event{
			ev("a", "p1", 30, "AC"), ev("b", "p1", 20, "AC"),
		}, []row{{"b", 1, 1, 20}, {"a", 2, 1, 30}}},
		{"ties share rank", []Event{
			ev("a", "p1", 10, "AC"), ev("b", "p1", 10, "AC"), ev("c", "p1", 50, "AC"),
		}, []row{{"a", 1, 1, 10}, {"b", 1, 1, 10}, {"c", 3, 1, 50}}},
		{"input order does not matter", []Event{
			ev("a", "p1", 30, "AC"), ev("a", "p1", 5, "WA"),
		}, []row{{"a", 1, 1, 50}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rows(Score(tt.events)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestScorePerProblem(t *testing.T) {
	got := Score([]Event{ev("a", "p2", 3, "WA"), ev("a", "p1", 9, "AC")})
	want := []ProblemResult{
		{ProblemSlug: "p1", Solved: true, SolvedAtMinutes: 9, PenaltyMinutes: 9},
		{ProblemSlug: "p2", Rejected: 1},
	}
	if len(got) != 1 || !reflect.DeepEqual(got[0].PerProblem, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestScoreDoesNotMutateInput(t *testing.T) {
	in := []Event{ev("a", "p1", 30, "AC"), ev("a", "p1", 5, "WA")}
	first := in[0]
	Score(in)
	if in[0] != first {
		t.Fatal("input reordered")
	}
}

func TestStatusAt(t *testing.T) {
	s := time.Unix(1000, 0)
	e := time.Unix(2000, 0)
	for _, tt := range []struct {
		at   int64
		want string
	}{{999, StatusUpcoming}, {1000, StatusRunning}, {1999, StatusRunning}, {2000, StatusEnded}} {
		if got := StatusAt(s, e, time.Unix(tt.at, 0)); got != tt.want {
			t.Errorf("at %d: %s, want %s", tt.at, got, tt.want)
		}
	}
}
