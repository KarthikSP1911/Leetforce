// Package validate checks a problem directory before it is published.
//
// Two steps, kept apart because only the second needs the sandbox:
//
//   - Structural checks (this file) read the files and run anywhere: spec
//     fields, statement, starters, test file pairs, sizes.
//   - The reference-solution check (reference.go) judges every file under
//     solutions/<lang>/<verdict>.<ext> and requires the verdict its name
//     promises. It needs root, nsjail and cgroup v2, like `judge run`.
package validate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"leetforce/judge/problem"
)

// Bounds a problem must stay within.
const (
	MinTests        = 3        // tests in total, samples included
	MinHidden       = 1        // tests that are not samples
	MaxTests        = 200      // more would make one submission judge for minutes
	MaxTestBytes    = 1 << 20  // one .in or .out file
	MaxTotalBytes   = 32 << 20 // all tests; the packed bundle must stay under problem.MaxBundleBytes
	MaxStatement    = 64 << 10
	MaxStarterBytes = 16 << 10
	MaxTimeMS       = 10_000
	MinMemoryMB     = 16
	MaxMemoryMB     = 1024
)

// Level says how serious an Issue is.
type Level string

// Issue levels. Errors fail validation; warnings do not, unless strict.
const (
	Error   Level = "error"
	Warning Level = "warning"
)

// Issue is one finding.
type Issue struct {
	Level Level
	Msg   string
}

func (i Issue) String() string { return string(i.Level) + ": " + i.Msg }

// Result collects the findings for one problem directory.
type Result struct {
	Dir    string
	Issues []Issue
}

func (r *Result) errorf(format string, a ...any) {
	r.Issues = append(r.Issues, Issue{Error, fmt.Sprintf(format, a...)})
}

func (r *Result) warnf(format string, a ...any) {
	r.Issues = append(r.Issues, Issue{Warning, fmt.Sprintf(format, a...)})
}

// Failed reports whether there is an error, or any finding when strict.
func (r *Result) Failed(strict bool) bool {
	for _, i := range r.Issues {
		if i.Level == Error || strict {
			return true
		}
	}
	return false
}

func (r *Result) hasError() bool {
	for _, i := range r.Issues {
		if i.Level == Error {
			return true
		}
	}
	return false
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Structure runs the structural checks on dir. It never needs the sandbox.
// It reads the files itself rather than calling problem.Load first, so one
// run reports every mistake instead of stopping at the first.
func Structure(dir string) *Result {
	r := &Result{Dir: dir}
	spec, ok := checkSpec(r, dir)
	if !ok {
		return r
	}
	checkTests(r, dir, spec)
	checkContent(r, dir)
	// Load applies the rules the runtime applies; anything it rejects that we
	// did not already report is still a failure.
	if !r.hasError() {
		if _, err := problem.Load(dir); err != nil {
			r.errorf("problem does not load: %v", err)
		}
	}
	return r
}

func checkSpec(r *Result, dir string) (problem.Spec, bool) {
	var spec problem.Spec
	raw, err := os.ReadFile(filepath.Join(dir, "problem.yaml")) //nolint:gosec // operator-chosen dir
	if err != nil {
		r.errorf("problem.yaml: %v", err)
		return spec, false
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&spec); err != nil {
		r.errorf("problem.yaml: %v", err)
		return spec, false
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	base := filepath.Base(abs)
	switch {
	case !slugRe.MatchString(spec.Slug):
		r.errorf("slug %q must be lowercase words joined by hyphens", spec.Slug)
	case spec.Slug != base:
		r.errorf("slug %q must equal the directory name %q", spec.Slug, base)
	}
	if strings.TrimSpace(spec.Title) == "" {
		r.errorf("title is required")
	}
	switch spec.Difficulty {
	case "easy", "medium", "hard":
	default:
		r.errorf("difficulty %q must be easy, medium or hard", spec.Difficulty)
	}
	switch spec.Checker {
	case "", problem.CheckerExact, problem.CheckerTokens:
	default:
		r.errorf("checker %q must be %q or %q", spec.Checker, problem.CheckerExact, problem.CheckerTokens)
	}
	if len(spec.Tags) == 0 {
		r.warnf("tags is empty; the problem list filters by tag")
	}
	checkLimit(r, "limits.default", spec.Limits.Default)
	for lang, l := range spec.Limits.Overrides {
		if !contains(problem.Languages, lang) {
			r.errorf("limits.overrides: unknown language %q", lang)
			continue
		}
		checkLimit(r, "limits.overrides."+lang, l)
	}
	return spec, true
}

func checkLimit(r *Result, name string, l problem.Limit) {
	if l.TimeMS <= 0 || l.TimeMS > MaxTimeMS {
		r.errorf("%s.time_ms %d must be between 1 and %d", name, l.TimeMS, MaxTimeMS)
	}
	if l.MemoryMB < MinMemoryMB || l.MemoryMB > MaxMemoryMB {
		r.errorf("%s.memory_mb %d must be between %d and %d", name, l.MemoryMB, MinMemoryMB, MaxMemoryMB)
	}
}

// checkTests looks at tests/ as a file listing: every NAME.in needs a
// NAME.out and the reverse, names are unique (also ignoring case), sample
// names exist, and there are enough hidden tests.
func checkTests(r *Result, dir string, spec problem.Spec) {
	entries, err := os.ReadDir(filepath.Join(dir, "tests"))
	if err != nil {
		r.errorf("tests/: %v", err)
		return
	}
	in, out := map[string]bool{}, map[string]bool{}
	lower := map[string]string{}
	var total int64
	for _, e := range entries {
		n := e.Name()
		var name string
		switch {
		case e.IsDir():
			r.errorf("tests/%s: directories are not allowed", n)
			continue
		case strings.HasSuffix(n, ".in"):
			name = strings.TrimSuffix(n, ".in")
			in[name] = true
		case strings.HasSuffix(n, ".out"):
			name = strings.TrimSuffix(n, ".out")
			out[name] = true
		default:
			r.errorf("tests/%s: only NAME.in and NAME.out files are allowed", n)
			continue
		}
		if prev, ok := lower[strings.ToLower(name)]; ok && prev != name {
			r.errorf("tests/: names %q and %q differ only by case", prev, name)
		}
		lower[strings.ToLower(name)] = name
		info, err := e.Info()
		if err != nil {
			r.errorf("tests/%s: %v", n, err)
			continue
		}
		total += info.Size()
		if info.Size() > MaxTestBytes {
			r.errorf("tests/%s is %d bytes, limit %d", n, info.Size(), MaxTestBytes)
		}
	}
	if total > MaxTotalBytes {
		r.errorf("tests/ holds %d bytes, limit %d", total, MaxTotalBytes)
	}
	names := map[string]bool{}
	for _, n := range sortedKeys(in) {
		names[n] = true
		if !out[n] {
			r.errorf("tests/%s.in has no %s.out", n, n)
		}
	}
	for _, n := range sortedKeys(out) {
		names[n] = true
		if !in[n] {
			r.errorf("tests/%s.out has no %s.in (orphan)", n, n)
		}
	}
	if len(names) < MinTests {
		r.errorf("%d tests, need at least %d", len(names), MinTests)
	}
	if len(names) > MaxTests {
		r.errorf("%d tests, limit %d", len(names), MaxTests)
	}
	seen := map[string]bool{}
	samples := 0
	for _, s := range spec.Samples {
		switch {
		case seen[s]:
			r.errorf("samples: %q is listed twice", s)
		case !names[s]:
			r.errorf("samples: no test named %q", s)
		default:
			samples++
		}
		seen[s] = true
	}
	if len(spec.Samples) == 0 {
		r.errorf("at least one sample test is required")
	}
	if hidden := len(names) - samples; hidden < MinHidden {
		r.errorf("%d hidden tests, need at least %d (samples are the only tests users see)", hidden, MinHidden)
	}
}

// checkContent checks statement.md and the starters. Missing ones are
// warnings, because a fixture such as sample-sum is judged but never shown;
// -strict turns warnings into failures.
func checkContent(r *Result, dir string) {
	raw, err := os.ReadFile(filepath.Join(dir, "statement.md")) //nolint:gosec // operator-chosen dir
	switch {
	case errors.Is(err, os.ErrNotExist):
		r.warnf("statement.md is missing")
	case err != nil:
		r.errorf("statement.md: %v", err)
	case strings.TrimSpace(string(raw)) == "":
		r.warnf("statement.md is empty")
	case len(raw) > MaxStatement:
		r.errorf("statement.md is %d bytes, limit %d", len(raw), MaxStatement)
	}
	for _, lang := range problem.Languages {
		file := problem.StarterFiles[lang]
		info, err := os.Stat(filepath.Join(dir, "starters", file))
		switch {
		case errors.Is(err, os.ErrNotExist):
			r.warnf("starters/%s is missing", file)
		case err != nil:
			r.errorf("starters/%s: %v", file, err)
		case info.Size() == 0:
			r.warnf("starters/%s is empty", file)
		case info.Size() > MaxStarterBytes:
			r.errorf("starters/%s is %d bytes, limit %d", file, info.Size(), MaxStarterBytes)
		}
	}
}

// Dirs returns the problem directories under root: root itself when it holds
// a problem.yaml, otherwise each immediate subdirectory that does, sorted.
func Dirs(root string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(root, "problem.yaml")); err == nil {
		return []string{root}, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("list problems: %w", err)
	}
	var dirs []string
	for _, e := range entries {
		d := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(d, "problem.yaml")); e.IsDir() && err == nil {
			dirs = append(dirs, d)
		}
	}
	sort.Strings(dirs)
	if len(dirs) == 0 {
		return nil, fmt.Errorf("no problem.yaml in %s or its subdirectories", root)
	}
	return dirs, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
