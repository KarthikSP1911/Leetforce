package server

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"leetforce/api/internal/catalog"
	"leetforce/api/internal/store"
)

type fakeContent struct {
	statements map[string]string
	starters   map[string]map[string]string
}

func (f fakeContent) Statement(slug string) string { return f.statements[slug] }
func (f fakeContent) Starters(slug string) map[string]string {
	m := map[string]string{}
	for k, v := range f.starters[slug] {
		m[k] = v
	}
	return m
}

func pct(v float64) *float64 { return &v }

func sampleProblems() []store.Problem {
	return []store.Problem{
		{Slug: "a-sum", Title: "Sum of Numbers", Difficulty: "easy", Tags: []string{"math", "array"}, Acceptance: pct(75.5)},
		{Slug: "b-rev", Title: "Reverse Words", Difficulty: "easy", Tags: []string{"string"}},
		{Slug: "c-max", Title: "Maximum Subarray", Difficulty: "medium", Tags: []string{"array", "dp"}, Acceptance: pct(0)},
		{Slug: "d-par", Title: "Balanced Brackets", Difficulty: "hard", Tags: []string{"string", "stack"}},
	}
}

type listBody struct {
	Problems []store.Problem `json:"problems"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

func TestListProblemsQuery(t *testing.T) {
	d := Deps{Problems: fakeProblems{list: sampleProblems()}}
	many := make([]store.Problem, 130)
	for i := range many {
		many[i] = store.Problem{Slug: fmt.Sprintf("p-%03d", i), Title: fmt.Sprintf("P %d", i), Difficulty: "easy"}
	}

	tests := []struct {
		name      string
		deps      Deps
		query     string
		wantCode  int
		wantSlugs []string
		wantTotal int
		wantPage  int
		wantSize  int
	}{
		{"defaults", d, "", 200, []string{"a-sum", "b-rev", "c-max", "d-par"}, 4, 1, 20},
		{"title substring is case-insensitive", d, "q=WORDS", 200, []string{"b-rev"}, 1, 1, 20},
		{"title matches several", d, "q=um", 200, []string{"a-sum", "c-max"}, 2, 1, 20},
		{"no match is an empty array", d, "q=zzz", 200, []string{}, 0, 1, 20},
		{"difficulty", d, "difficulty=easy", 200, []string{"a-sum", "b-rev"}, 2, 1, 20},
		{"difficulty is case-insensitive", d, "difficulty=HARD", 200, []string{"d-par"}, 1, 1, 20},
		{"bad difficulty", d, "difficulty=extreme", 400, nil, 0, 0, 0},
		{"single tag", d, "tag=array", 200, []string{"a-sum", "c-max"}, 2, 1, 20},
		{"repeated tags must all match", d, "tag=array&tag=dp", 200, []string{"c-max"}, 1, 1, 20},
		{"tag is exact", d, "tag=arr", 200, []string{}, 0, 1, 20},
		{"filters combine", d, "difficulty=easy&tag=string&q=rev", 200, []string{"b-rev"}, 1, 1, 20},
		{"page size", d, "page_size=2", 200, []string{"a-sum", "b-rev"}, 4, 1, 2},
		{"second page", d, "page=2&page_size=3", 200, []string{"d-par"}, 4, 2, 3},
		{"page past the end", d, "page=9&page_size=3", 200, []string{}, 4, 9, 3},
		{"page size is capped", Deps{Problems: fakeProblems{list: many}}, "page_size=1000", 200, nil, 130, 1, 100},
		{"default page size is 20", Deps{Problems: fakeProblems{list: many}}, "", 200, nil, 130, 1, 20},
		{"page zero", d, "page=0", 400, nil, 0, 0, 0},
		{"page not a number", d, "page=x", 400, nil, 0, 0, 0},
		{"page_size negative", d, "page_size=-1", 400, nil, 0, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := get(t, tc.deps, "/problems?"+tc.query)
			if w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.wantCode, w.Body)
			}
			if tc.wantCode != 200 {
				return
			}
			var body listBody
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Total != tc.wantTotal || body.Page != tc.wantPage || body.PageSize != tc.wantSize {
				t.Errorf("total/page/size = %d/%d/%d, want %d/%d/%d", body.Total, body.Page, body.PageSize, tc.wantTotal, tc.wantPage, tc.wantSize)
			}
			if tc.wantSlugs != nil {
				got := []string{}
				for _, p := range body.Problems {
					got = append(got, p.Slug)
				}
				if strings.Join(got, ",") != strings.Join(tc.wantSlugs, ",") {
					t.Errorf("slugs = %v, want %v", got, tc.wantSlugs)
				}
			} else if tc.wantSize == 100 && len(body.Problems) != 100 {
				t.Errorf("returned %d problems, want 100", len(body.Problems))
			}
			if !strings.Contains(w.Body.String(), `"problems":[`) {
				t.Errorf("problems must be a JSON array: %s", w.Body)
			}
		})
	}
}

func TestListProblemsAcceptanceJSON(t *testing.T) {
	d := Deps{Problems: fakeProblems{list: sampleProblems()}}
	w := get(t, d, "/problems")
	var raw struct {
		Problems []map[string]any `json:"problems"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	want := []any{75.5, nil, 0.0, nil}
	for i, p := range raw.Problems {
		v, present := p["acceptance"]
		if !present || v != want[i] {
			t.Errorf("%v: acceptance = %v (present %v), want %v", p["slug"], v, present, want[i])
		}
	}
	if strings.Contains(w.Body.String(), "test_set") {
		t.Errorf("list leaks the test-set version: %s", w.Body)
	}
}

func TestProblemDetailContent(t *testing.T) {
	hidden := "HIDDEN-INPUT-9731"
	d := Deps{
		Problems: fakeProblems{list: sampleProblems()},
		Samples:  fakeSamples{"a-sum": {{Name: "01", Input: "1 2\n", Expected: "3\n"}}},
		Content: fakeContent{
			statements: map[string]string{"a-sum": "# Sum\n\nAdd them."},
			starters:   map[string]map[string]string{"a-sum": {"python": "print(0)\n", "go": "package main\n"}},
		},
	}
	w := get(t, d, "/problems/a-sum")
	if w.Code != 200 {
		t.Fatalf("status = %d: %s", w.Code, w.Body)
	}
	var body struct {
		Slug       string
		Acceptance *float64
		Statement  string
		Starters   map[string]string
		Samples    []catalog.Sample
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Statement != "# Sum\n\nAdd them." || body.Starters["python"] != "print(0)\n" || len(body.Starters) != 2 {
		t.Errorf("content = %q %v", body.Statement, body.Starters)
	}
	if body.Acceptance == nil || *body.Acceptance != 75.5 || len(body.Samples) != 1 {
		t.Errorf("detail = %+v", body)
	}
	if strings.Contains(w.Body.String(), hidden) || strings.Contains(w.Body.String(), "test_set") {
		t.Errorf("detail leaks hidden data: %s", w.Body)
	}

	t.Run("problem without content has empty statement and starters", func(t *testing.T) {
		w := get(t, d, "/problems/b-rev")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"statement":""`) ||
			!strings.Contains(w.Body.String(), `"starters":{}`) || !strings.Contains(w.Body.String(), `"samples":[]`) {
			t.Fatalf("detail = %d %s", w.Code, w.Body)
		}
	})
	t.Run("nil Content is tolerated", func(t *testing.T) {
		w := get(t, Deps{Problems: fakeProblems{list: sampleProblems()}, Samples: fakeSamples{}}, "/problems/a-sum")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"starters":{}`) {
			t.Fatalf("detail = %d %s", w.Code, w.Body)
		}
	})
	t.Run("unknown slug is 404", func(t *testing.T) {
		if w := get(t, d, "/problems/"+url.PathEscape("nope")); w.Code != 404 {
			t.Fatalf("status = %d", w.Code)
		}
	})
}

// With the real catalog, the detail response carries the statement and
// starters of a repo problem and none of its hidden test data.
func TestProblemDetailFromRepoCatalog(t *testing.T) {
	cat, err := catalog.Load("../../../problems")
	if err != nil {
		t.Fatal(err)
	}
	p := cat.Problems()
	var stored []store.Problem
	for _, x := range p {
		stored = append(stored, store.Problem{Slug: x.Spec.Slug, Title: x.Spec.Title, Difficulty: x.Spec.Difficulty, Tags: x.Spec.Tags})
	}
	d := Deps{Problems: fakeProblems{list: stored}, Samples: cat, Content: cat}
	for _, x := range p {
		if x.Spec.Slug == "sample-sum" {
			continue
		}
		w := get(t, d, "/problems/"+x.Spec.Slug)
		if w.Code != 200 {
			t.Fatalf("%s: status %d", x.Spec.Slug, w.Code)
		}
		var body struct {
			Statement string
			Starters  map[string]string
			Samples   []catalog.Sample
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Statement == "" || len(body.Starters) != 4 || len(body.Samples) != 2 {
			t.Errorf("%s: statement %d bytes, %d starters, %d samples", x.Spec.Slug, len(body.Statement), len(body.Starters), len(body.Samples))
		}
		for _, tc := range x.Tests {
			if tc.Sample || len(tc.Input) < 8 {
				continue // samples are meant to be visible; tiny inputs would match by chance
			}
			if strings.Contains(w.Body.String(), strings.TrimSpace(string(tc.Input))) {
				t.Errorf("%s: response contains hidden test %s input", x.Spec.Slug, tc.Name)
			}
		}
	}
}
