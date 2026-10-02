// Package problem loads and validates problem definitions (problem.yaml plus
// a tests/ directory) and computes the test-set version.
package problem

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Checker names how a program's output is compared with the expected output.
const (
	CheckerExact  = "exact"  // byte-for-byte, except one trailing newline is ignored
	CheckerTokens = "tokens" // whitespace-separated tokens must match
)

// Languages the judge supports.
var Languages = []string{"python", "cpp", "java", "go"}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Limit is a time and memory limit for one language.
type Limit struct {
	TimeMS   int `yaml:"time_ms"`
	MemoryMB uint64 `yaml:"memory_mb"`
}

// Time returns the time limit as a duration.
func (l Limit) Time() time.Duration { return time.Duration(l.TimeMS) * time.Millisecond }

// MemoryBytes returns the memory limit in bytes.
func (l Limit) MemoryBytes() uint64 { return l.MemoryMB << 20 }

// Limits holds the default limit and optional per-language overrides, so a
// slow-starting runtime such as the JVM can be given more room.
type Limits struct {
	Default   Limit            `yaml:"default"`
	Overrides map[string]Limit `yaml:"overrides"`
}

// Spec is the parsed problem.yaml.
type Spec struct {
	Slug       string   `yaml:"slug"`
	Title      string   `yaml:"title"`
	Difficulty string   `yaml:"difficulty"`
	Tags       []string `yaml:"tags"`
	Checker    string   `yaml:"checker"`
	Limits     Limits   `yaml:"limits"`
	// Samples names the tests (file name without extension) that are visible
	// to users and may be shown by Run. All other tests are hidden.
	Samples []string `yaml:"samples"`
}

// Test is one input/expected-output pair.
type Test struct {
	Name     string
	Input    []byte
	Expected []byte
	Sample   bool
}

// Problem is a loaded problem: its spec, its tests in name order, and the
// version of that test set.
type Problem struct {
	Spec       Spec
	Tests      []Test
	TestSetVer string
	Dir        string
}

// LimitFor returns the limit for a language: the override if present,
// otherwise the default.
func (s Spec) LimitFor(lang string) Limit {
	if l, ok := s.Limits.Overrides[lang]; ok {
		return l
	}
	return s.Limits.Default
}

// Load reads <dir>/problem.yaml and <dir>/tests/*.in|*.out, validates them and
// computes the test-set version.
func Load(dir string) (*Problem, error) {
	path := filepath.Join(dir, "problem.yaml")
	raw, err := os.ReadFile(path) //nolint:gosec // dir is chosen by the operator
	if err != nil {
		return nil, fmt.Errorf("load problem: %w", err)
	}
	var spec Spec
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if spec.Checker == "" {
		spec.Checker = CheckerTokens
	}
	tests, err := loadTests(filepath.Join(dir, "tests"), spec.Samples)
	if err != nil {
		return nil, err
	}
	p := &Problem{Spec: spec, Tests: tests, Dir: dir}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	p.TestSetVer = Version(spec.Checker, tests)
	return p, nil
}

func loadTests(dir string, samples []string) ([]Test, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("load tests: %w", err)
	}
	isSample := make(map[string]bool, len(samples))
	for _, s := range samples {
		isSample[s] = true
	}
	names := map[string]bool{}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || (!strings.HasSuffix(n, ".in") && !strings.HasSuffix(n, ".out")) {
			return nil, fmt.Errorf("load tests: unexpected entry %q (only NAME.in and NAME.out files are allowed)", n)
		}
		names[strings.TrimSuffix(strings.TrimSuffix(n, ".in"), ".out")] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	tests := make([]Test, 0, len(sorted))
	for _, n := range sorted {
		in, err := os.ReadFile(filepath.Join(dir, n+".in")) //nolint:gosec // operator-chosen dir
		if err != nil {
			return nil, fmt.Errorf("load test %q input: %w", n, err)
		}
		out, err := os.ReadFile(filepath.Join(dir, n+".out")) //nolint:gosec // operator-chosen dir
		if err != nil {
			return nil, fmt.Errorf("load test %q expected output: %w", n, err)
		}
		tests = append(tests, Test{Name: n, Input: in, Expected: out, Sample: isSample[n]})
	}
	for s := range isSample {
		if !names[s] {
			return nil, fmt.Errorf("samples: no test named %q", s)
		}
	}
	return tests, nil
}

// Validate checks the spec and tests for mistakes an author could make.
func (p *Problem) Validate() error {
	s := p.Spec
	var errs []error
	if !slugRe.MatchString(s.Slug) {
		errs = append(errs, fmt.Errorf("slug %q must be lowercase words joined by hyphens", s.Slug))
	}
	if strings.TrimSpace(s.Title) == "" {
		errs = append(errs, errors.New("title is required"))
	}
	switch s.Difficulty {
	case "easy", "medium", "hard":
	default:
		errs = append(errs, fmt.Errorf("difficulty %q must be easy, medium or hard", s.Difficulty))
	}
	switch s.Checker {
	case CheckerExact, CheckerTokens:
	default:
		errs = append(errs, fmt.Errorf("checker %q must be %q or %q", s.Checker, CheckerExact, CheckerTokens))
	}
	if s.Limits.Default.TimeMS <= 0 || s.Limits.Default.MemoryMB == 0 {
		errs = append(errs, errors.New("limits.default needs positive time_ms and memory_mb"))
	}
	for lang, l := range s.Limits.Overrides {
		if !isLanguage(lang) {
			errs = append(errs, fmt.Errorf("limits.overrides: unknown language %q", lang))
		}
		if l.TimeMS <= 0 || l.MemoryMB == 0 {
			errs = append(errs, fmt.Errorf("limits.overrides.%s needs positive time_ms and memory_mb", lang))
		}
	}
	if len(p.Tests) == 0 {
		errs = append(errs, errors.New("at least one test is required"))
	}
	if len(s.Samples) == 0 {
		errs = append(errs, errors.New("at least one sample test is required"))
	}
	return errors.Join(errs...)
}

func isLanguage(l string) bool {
	for _, x := range Languages {
		if x == l {
			return true
		}
	}
	return false
}
