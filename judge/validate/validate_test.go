package validate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"leetforce/judge/engine"
	"leetforce/judge/problem"
	"leetforce/judge/verdict"
)

const goodYAML = `slug: demo
title: Demo
difficulty: easy
tags: [math]
limits:
  default:
    time_ms: 1000
    memory_mb: 128
samples: [01]
`

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// newProblem writes a valid problem named demo and returns its directory.
func newProblem(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "demo")
	write(t, dir, "problem.yaml", goodYAML)
	write(t, dir, "statement.md", "# Demo\n")
	for _, f := range problem.StarterFiles {
		write(t, dir, "starters/"+f, "// start\n")
	}
	for _, n := range []string{"01", "02", "03"} {
		write(t, dir, "tests/"+n+".in", n+"\n")
		write(t, dir, "tests/"+n+".out", n+"\n")
	}
	write(t, dir, "solutions/python/ac.py", "print(1)\n")
	return dir
}

func messages(r *Result, lvl Level) string {
	var b strings.Builder
	for _, i := range r.Issues {
		if i.Level == lvl {
			b.WriteString(i.Msg + "\n")
		}
	}
	return b.String()
}

func TestStructureValid(t *testing.T) {
	r := Structure(newProblem(t))
	if len(r.Issues) != 0 {
		t.Fatalf("issues on a valid problem: %v", r.Issues)
	}
	if r.Failed(true) {
		t.Fatal("valid problem failed")
	}
}

func TestStructureFindings(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(t *testing.T, dir string)
		want    string
		level   Level
		strict  bool
		failure bool
	}{
		{"slug differs from directory", func(t *testing.T, d string) {
			write(t, d, "problem.yaml", strings.Replace(goodYAML, "slug: demo", "slug: other", 1))
		}, "must equal the directory name", Error, false, true},
		{"bad difficulty", func(t *testing.T, d string) {
			write(t, d, "problem.yaml", strings.Replace(goodYAML, "easy", "trivial", 1))
		}, "difficulty", Error, false, true},
		{"missing title", func(t *testing.T, d string) {
			write(t, d, "problem.yaml", strings.Replace(goodYAML, "title: Demo", "title: ' '", 1))
		}, "title is required", Error, false, true},
		{"time limit too large", func(t *testing.T, d string) {
			write(t, d, "problem.yaml", strings.Replace(goodYAML, "time_ms: 1000", "time_ms: 600000", 1))
		}, "time_ms", Error, false, true},
		{"memory limit too small", func(t *testing.T, d string) {
			write(t, d, "problem.yaml", strings.Replace(goodYAML, "memory_mb: 128", "memory_mb: 1", 1))
		}, "memory_mb", Error, false, true},
		{"unknown override language", func(t *testing.T, d string) {
			write(t, d, "problem.yaml", goodYAML+"")
			write(t, d, "problem.yaml", strings.Replace(goodYAML, "samples", "  overrides:\n    rust:\n      time_ms: 1\n      memory_mb: 64\nsamples", 1))
		}, "unknown language", Error, false, true},
		{"unknown yaml field", func(t *testing.T, d string) {
			write(t, d, "problem.yaml", goodYAML+"colour: red\n")
		}, "colour", Error, false, true},
		{"orphan expected output", func(t *testing.T, d string) {
			write(t, d, "tests/04.out", "x\n")
		}, "04.out has no 04.in", Error, false, true},
		{"input without output", func(t *testing.T, d string) {
			write(t, d, "tests/04.in", "x\n")
		}, "04.in has no 04.out", Error, false, true},
		{"stray file in tests", func(t *testing.T, d string) {
			write(t, d, "tests/notes.txt", "x\n")
		}, "only NAME.in and NAME.out", Error, false, true},
		{"names differing by case", func(t *testing.T, d string) {
			write(t, d, "tests/A.in", "x\n")
			write(t, d, "tests/a.in", "x\n")
			write(t, d, "tests/A.out", "x\n")
			write(t, d, "tests/a.out", "x\n")
		}, "differ only by case", Error, false, true},
		{"oversized test", func(t *testing.T, d string) {
			write(t, d, "tests/03.in", strings.Repeat("x", MaxTestBytes+1))
		}, "03.in is", Error, false, true},
		{"too few tests", func(t *testing.T, d string) {
			for _, f := range []string{"02.in", "02.out", "03.in", "03.out"} {
				if err := os.Remove(filepath.Join(d, "tests", f)); err != nil {
					t.Fatal(err)
				}
			}
		}, "need at least 3", Error, false, true},
		{"sample names a missing test", func(t *testing.T, d string) {
			write(t, d, "problem.yaml", strings.Replace(goodYAML, "[01]", "[01, 09]", 1))
		}, `no test named "09"`, Error, false, true},
		{"duplicate sample", func(t *testing.T, d string) {
			write(t, d, "problem.yaml", strings.Replace(goodYAML, "[01]", "[01, 01]", 1))
		}, "listed twice", Error, false, true},
		{"no hidden tests", func(t *testing.T, d string) {
			write(t, d, "problem.yaml", strings.Replace(goodYAML, "[01]", "[01, 02, 03]", 1))
		}, "hidden tests", Error, false, true},
		{"missing statement is a warning", func(t *testing.T, d string) {
			if err := os.Remove(filepath.Join(d, "statement.md")); err != nil {
				t.Fatal(err)
			}
		}, "statement.md is missing", Warning, false, false},
		{"missing statement fails strict", func(t *testing.T, d string) {
			if err := os.Remove(filepath.Join(d, "statement.md")); err != nil {
				t.Fatal(err)
			}
		}, "statement.md is missing", Warning, true, true},
		{"missing starter is a warning", func(t *testing.T, d string) {
			if err := os.Remove(filepath.Join(d, "starters", "go.go")); err != nil {
				t.Fatal(err)
			}
		}, "starters/go.go is missing", Warning, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := newProblem(t)
			tc.mutate(t, dir)
			r := Structure(dir)
			if got := messages(r, tc.level); !strings.Contains(got, tc.want) {
				t.Fatalf("%s findings = %q, want one containing %q", tc.level, got, tc.want)
			}
			if r.Failed(tc.strict) != tc.failure {
				t.Fatalf("Failed(strict=%v) = %v, want %v (%v)", tc.strict, r.Failed(tc.strict), tc.failure, r.Issues)
			}
		})
	}
}

func TestDirs(t *testing.T) {
	root := t.TempDir()
	write(t, root, "b/problem.yaml", "x")
	write(t, root, "a/problem.yaml", "x")
	write(t, root, "c/readme", "x")
	got, err := Dirs(root)
	if err != nil || len(got) != 2 || filepath.Base(got[0]) != "a" || filepath.Base(got[1]) != "b" {
		t.Fatalf("Dirs(root) = %v, %v", got, err)
	}
	got, err = Dirs(filepath.Join(root, "a"))
	if err != nil || len(got) != 1 {
		t.Fatalf("Dirs(single) = %v, %v", got, err)
	}
	if _, err := Dirs(filepath.Join(root, "c")); err == nil {
		t.Fatal("Dirs on a directory without problems should fail")
	}
}

// fakeJudger returns a canned verdict per solution file name.
type fakeJudger struct {
	byLang map[string]verdict.Verdict
	err    error
	calls  []string
}

func (f *fakeJudger) Judge(_ context.Context, _ *problem.Problem, lang string, src []byte, _ engine.Options) (*engine.Report, error) {
	f.calls = append(f.calls, lang+":"+strings.TrimSpace(string(src)))
	if f.err != nil {
		return nil, f.err
	}
	v := f.byLang[strings.TrimSpace(string(src))]
	rep := &engine.Report{Overall: verdict.Overall{Verdict: v}}
	if v != verdict.AC {
		rep.Overall.Failed = "02"
	}
	return rep, nil
}

func loaded(t *testing.T, dir string) *problem.Problem {
	t.Helper()
	p, err := problem.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSolutionsNaming(t *testing.T) {
	dir := newProblem(t)
	write(t, dir, "solutions/python/wa.py", "wa\n")
	write(t, dir, "solutions/python/notes.txt", "x\n")
	write(t, dir, "solutions/python/tle.cpp", "x\n") // wrong extension for the directory
	write(t, dir, "solutions/rust/ac.rs", "x\n")
	sols, issues := Solutions(dir)
	if len(sols) != 2 {
		t.Fatalf("solutions = %+v, want ac.py and wa.py", sols)
	}
	if len(issues) != 3 {
		t.Fatalf("issues = %v, want 3 (notes.txt, tle.cpp, rust/)", issues)
	}
	if sols[0].Expected != verdict.AC || sols[1].Expected != verdict.WA {
		t.Fatalf("expected verdicts = %v, %v", sols[0].Expected, sols[1].Expected)
	}
}

func TestReference(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string // relative path under solutions -> source (also the fake's key)
		got   map[string]verdict.Verdict
		want  string // substring of the single expected error; "" means none
	}{
		{"ac and wa behave", map[string]string{"python/ac.py": "good", "python/wa.py": "bad"},
			map[string]verdict.Verdict{"good": verdict.AC, "bad": verdict.WA}, ""},
		{"ac gets WA", map[string]string{"python/ac.py": "good"},
			map[string]verdict.Verdict{"good": verdict.WA}, "expected AC, got WA"},
		{"wa gets AC", map[string]string{"python/ac.py": "good", "python/wa.py": "bad"},
			map[string]verdict.Verdict{"good": verdict.AC, "bad": verdict.AC}, "wa.py: expected WA, got AC"},
		{"language without ac", map[string]string{"python/ac.py": "good", "go/wa.go": "bad"},
			map[string]verdict.Verdict{"good": verdict.AC, "bad": verdict.WA}, "solutions/go has no ac"},
		{"no solutions at all", map[string]string{},
			nil, "no ac.<ext>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := newProblem(t)
			if err := os.RemoveAll(filepath.Join(dir, "solutions")); err != nil {
				t.Fatal(err)
			}
			if len(tc.files) > 0 {
				for rel, src := range tc.files {
					write(t, dir, "solutions/"+rel, src+"\n")
				}
			} else {
				if err := os.MkdirAll(filepath.Join(dir, "solutions"), 0o750); err != nil {
					t.Fatal(err)
				}
			}
			issues, err := Reference(context.Background(), &fakeJudger{byLang: tc.got}, loaded(t, dir))
			if err != nil {
				t.Fatal(err)
			}
			r := &Result{Issues: issues}
			got := messages(r, Error)
			if tc.want == "" && got != "" {
				t.Fatalf("unexpected errors: %s", got)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Fatalf("errors = %q, want one containing %q", got, tc.want)
			}
		})
	}
}

func TestReferenceHostErrorIsNotAnIssue(t *testing.T) {
	dir := newProblem(t)
	boom := errors.New("sandbox down")
	_, err := Reference(context.Background(), &fakeJudger{err: boom}, loaded(t, dir))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap the host error", err)
	}
}
