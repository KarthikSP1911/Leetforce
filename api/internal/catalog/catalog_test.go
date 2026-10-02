package catalog

import (
	"testing"
)

// The repository's own problems directory must load, and only the tests the
// problem marks as samples may come out of Samples.
func TestLoadRepoProblems(t *testing.T) {
	c, err := Load("../../../problems")
	if err != nil {
		t.Fatal(err)
	}
	ps := c.Problems()
	if len(ps) == 0 {
		t.Fatal("no problems loaded")
	}
	for _, p := range ps {
		want := 0
		for _, tc := range p.Tests {
			if tc.Sample {
				want++
			}
		}
		if len(p.Tests) <= want {
			t.Fatalf("%s: expected hidden tests besides the samples", p.Spec.Slug)
		}
		got := c.Samples(p.Spec.Slug)
		if len(got) != want {
			t.Fatalf("%s: Samples returned %d tests, want only the %d samples (of %d tests)", p.Spec.Slug, len(got), want, len(p.Tests))
		}
	}
	if c.Samples("does-not-exist") != nil {
		t.Fatal("unknown slug must have no samples")
	}
}

func TestLoadRejectsMissingDir(t *testing.T) {
	if _, err := Load("no/such/dir"); err == nil {
		t.Fatal("want an error for a missing directory")
	}
}
