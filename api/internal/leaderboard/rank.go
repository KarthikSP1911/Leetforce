package leaderboard

import (
	"sort"
	"strings"
	"time"
)

// Global score weights per distinct solved problem (ADR 0025).
const (
	WeightEasy   = 1
	WeightMedium = 3
	WeightHard   = 5
)

// RankGlobal orders users by weighted score, then solved count, then who
// reached it first (earlier last first-AC), then username. Users with equal
// score, solved count and time share a rank (competition ranking: 1, 2, 2, 4).
func RankGlobal(rows []GlobalRow) []GlobalEntry {
	out := make([]GlobalEntry, 0, len(rows))
	for _, r := range rows {
		e := GlobalEntry{
			UserID: r.UserID, Username: r.Username, Easy: r.Easy, Medium: r.Medium, Hard: r.Hard,
			Solved: r.Easy + r.Medium + r.Hard,
			Score:  r.Easy*WeightEasy + r.Medium*WeightMedium + r.Hard*WeightHard,
		}
		if !r.LastAC.IsZero() {
			t := r.LastAC.UTC()
			e.LastACAt = &t
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Solved != b.Solved {
			return a.Solved > b.Solved
		}
		if c := cmpTime(a.LastACAt, b.LastACAt); c != 0 {
			return c < 0
		}
		if c := strings.Compare(strings.ToLower(a.Username), strings.ToLower(b.Username)); c != 0 {
			return c < 0
		}
		return a.UserID < b.UserID
	})
	for i := range out {
		out[i].Rank = i + 1
		if i > 0 {
			p := out[i-1]
			if p.Score == out[i].Score && p.Solved == out[i].Solved && cmpTime(p.LastACAt, out[i].LastACAt) == 0 {
				out[i].Rank = p.Rank
			}
		}
	}
	return out
}

// cmpTime orders nil (never solved) after any time.
func cmpTime(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	return a.Compare(*b)
}

// epoch is the origin of rebased event times handed to the Scorer.
var epoch = time.Unix(0, 0).UTC()

// scoreEvents turns raw events into ranked standings.
//
// Events outside [start, end] are dropped here (a submission exactly at the
// end time still counts), the rest are rebased to offsets from the Unix epoch
// and handed to score for the ICPC totals. The per-problem cells and the
// last-AC tie-break are computed here from the same events, with the same
// rules (CE and IE ignored, nothing after the first AC counts).
func scoreEvents(c Contest, events []Event, names map[string]string, score Scorer) []StandingRow {
	in := make([]Event, 0, len(events))
	for _, e := range events {
		if e.SubmittedAt.Before(c.StartsAt) || e.SubmittedAt.After(c.EndsAt) {
			continue
		}
		in = append(in, e)
	}
	sort.SliceStable(in, func(i, j int) bool { return in[i].SubmittedAt.Before(in[j].SubmittedAt) })

	rebased := make([]Event, len(in))
	rows := map[string]*StandingRow{}
	for i, e := range in {
		r := e
		r.SubmittedAt = epoch.Add(e.SubmittedAt.Sub(c.StartsAt))
		rebased[i] = r

		row := rows[e.UserID]
		if row == nil {
			row = &StandingRow{UserID: e.UserID, Username: names[e.UserID], Cells: map[string]Cell{}}
			rows[e.UserID] = row
		}
		cell := row.Cells[e.ProblemSlug]
		if cell.Solved || e.Verdict == "CE" || e.Verdict == "IE" {
			continue
		}
		if e.Verdict == "AC" {
			cell.Solved = true
			cell.Minutes = int(e.SubmittedAt.Sub(c.StartsAt) / time.Minute)
			t := e.SubmittedAt.UTC()
			if row.LastACAt == nil || t.After(*row.LastACAt) {
				row.LastACAt = &t
			}
		} else {
			cell.Attempts++
		}
		row.Cells[e.ProblemSlug] = cell
	}

	for _, s := range score(rebased) {
		if row := rows[s.UserID]; row != nil {
			row.Solved, row.PenaltyMinutes = s.Solved, s.PenaltyMinutes
		}
	}
	out := make([]StandingRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Solved != b.Solved {
			return a.Solved > b.Solved
		}
		if a.PenaltyMinutes != b.PenaltyMinutes {
			return a.PenaltyMinutes < b.PenaltyMinutes
		}
		if c := cmpTime(a.LastACAt, b.LastACAt); c != 0 {
			return c < 0
		}
		if c := strings.Compare(strings.ToLower(a.Username), strings.ToLower(b.Username)); c != 0 {
			return c < 0
		}
		return a.UserID < b.UserID
	})
	for i := range out {
		out[i].Rank = i + 1
		if i > 0 {
			p := out[i-1]
			if p.Solved == out[i].Solved && p.PenaltyMinutes == out[i].PenaltyMinutes && cmpTime(p.LastACAt, out[i].LastACAt) == 0 {
				out[i].Rank = p.Rank
			}
		}
	}
	return out
}
