// Package catalog loads the problems directory for the API. Problem metadata
// is synced into Postgres; the visible sample tests are served from the
// loaded files. Hidden tests are loaded (to compute the test-set version) but
// never exposed: Samples returns only tests the problem marks as samples.
package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"leetforce/judge/problem"
)

// Sample is a visible example: the only test data the API ever returns.
type Sample struct {
	Name     string `json:"name"`
	Input    string `json:"input"`
	Expected string `json:"expected"`
}

// Catalog holds the loaded problems by slug.
type Catalog struct {
	problems map[string]*problem.Problem
}

// Load reads every <dir>/<slug>/problem.yaml. A directory without a
// problem.yaml is skipped; a problem that fails validation is an error, so a
// broken problem stops startup instead of being half served.
func Load(dir string) (*Catalog, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read problems dir: %w", err)
	}
	c := &Catalog{problems: map[string]*problem.Problem{}}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pdir := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(pdir, "problem.yaml")); err != nil {
			continue
		}
		p, err := problem.Load(pdir)
		if err != nil {
			return nil, fmt.Errorf("problem %q: %w", e.Name(), err)
		}
		if p.Spec.Slug != e.Name() {
			return nil, fmt.Errorf("problem %q: slug %q does not match its directory", e.Name(), p.Spec.Slug)
		}
		c.problems[p.Spec.Slug] = p
	}
	return c, nil
}

// Problems returns all problems ordered by slug.
func (c *Catalog) Problems() []*problem.Problem {
	out := make([]*problem.Problem, 0, len(c.problems))
	for _, p := range c.problems {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Spec.Slug < out[j].Spec.Slug })
	return out
}

// Samples returns the visible sample tests of a problem (nil if unknown).
func (c *Catalog) Samples(slug string) []Sample {
	p, ok := c.problems[slug]
	if !ok {
		return nil
	}
	var out []Sample
	for _, t := range p.Tests {
		if t.Sample {
			out = append(out, Sample{Name: t.Name, Input: string(t.Input), Expected: string(t.Expected)})
		}
	}
	return out
}
