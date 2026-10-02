package problem

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = `slug: demo-problem
title: Demo
difficulty: easy
tags: [math]
limits:
  default: {time_ms: 1000, memory_mb: 128}
  overrides:
    java: {time_ms: 2000, memory_mb: 256}
samples: [01]
`

// writeProblem creates a problem directory with the given yaml and test files.
func writeProblem(t *testing.T, yamlText string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tests"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "problem.yaml"), []byte(yamlText), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, "tests", name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

var okFiles = map[string]string{"01.in": "1\n", "01.out": "1\n", "02.in": "2\n", "02.out": "2\n"}

func TestLoadValid(t *testing.T) {
	p, err := Load(writeProblem(t, validYAML, okFiles))
	if err != nil {
		t.Fatal(err)
	}
	if p.Spec.Checker != CheckerTokens {
		t.Errorf("checker default = %q, want %q", p.Spec.Checker, CheckerTokens)
	}
	if len(p.Tests) != 2 || p.Tests[0].Name != "01" || !p.Tests[0].Sample || p.Tests[1].Sample {
		t.Errorf("tests = %+v", p.Tests)
	}
	if !strings.HasPrefix(p.TestSetVer, "ts-") || len(p.TestSetVer) != 19 {
		t.Errorf("version = %q", p.TestSetVer)
	}
}

func TestLimitFor(t *testing.T) {
	p, err := Load(writeProblem(t, validYAML, okFiles))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		lang   string
		wantMS int
		wantMB uint64
	}{
		{"python", 1000, 128},
		{"java", 2000, 256},
	}
	for _, tc := range tests {
		l := p.Spec.LimitFor(tc.lang)
		if l.TimeMS != tc.wantMS || l.MemoryMB != tc.wantMB {
			t.Errorf("LimitFor(%s) = %+v, want %dms/%dMB", tc.lang, l, tc.wantMS, tc.wantMB)
		}
	}
	if got := p.Spec.LimitFor("java").MemoryBytes(); got != 256<<20 {
		t.Errorf("MemoryBytes = %d", got)
	}
}

func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		files map[string]string
		want  string
	}{
		{"bad slug", strings.Replace(validYAML, "demo-problem", "Demo_Problem", 1), okFiles, "slug"},
		{"bad difficulty", strings.Replace(validYAML, "easy", "trivial", 1), okFiles, "difficulty"},
		{"bad checker", validYAML + "checker: fuzzy\n", okFiles, "checker"},
		{"zero limit", strings.Replace(validYAML, "time_ms: 1000", "time_ms: 0", 1), okFiles, "limits.default"},
		{"negative memory", strings.Replace(validYAML, "memory_mb: 128", "memory_mb: -1", 1), okFiles, "cannot unmarshal"},
		{"unknown language", strings.Replace(validYAML, "java:", "cobol:", 1), okFiles, "unknown language"},
		{"unknown field", validYAML + "colour: blue\n", okFiles, "colour"},
		{"missing sample test", strings.Replace(validYAML, "[01]", "[09]", 1), okFiles, `no test named "09"`},
		{"no samples", strings.Replace(validYAML, "samples: [01]", "samples: []", 1), okFiles, "sample"},
		{"missing output", validYAML, map[string]string{"01.in": "1\n"}, "expected output"},
		{"stray file", validYAML, map[string]string{"01.in": "1\n", "01.out": "1\n", "notes.txt": "x"}, "unexpected entry"},
		{"no tests", validYAML, nil, "sample"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(writeProblem(t, tc.yaml, tc.files))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestLoadMissingDir(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

func TestVersion(t *testing.T) {
	base := []Test{{Name: "01", Input: []byte("a"), Expected: []byte("b")}}
	v := Version(CheckerTokens, base)
	tests := []struct {
		name      string
		checker   string
		tests     []Test
		wantEqual bool
	}{
		{"same", CheckerTokens, []Test{{Name: "01", Input: []byte("a"), Expected: []byte("b")}}, true},
		{"sample flag ignored", CheckerTokens, []Test{{Name: "01", Input: []byte("a"), Expected: []byte("b"), Sample: true}}, true},
		{"input changed", CheckerTokens, []Test{{Name: "01", Input: []byte("c"), Expected: []byte("b")}}, false},
		{"expected changed", CheckerTokens, []Test{{Name: "01", Input: []byte("a"), Expected: []byte("c")}}, false},
		{"renamed", CheckerTokens, []Test{{Name: "02", Input: []byte("a"), Expected: []byte("b")}}, false},
		{"checker changed", CheckerExact, base, false},
		{"bytes moved between fields", CheckerTokens, []Test{{Name: "01", Input: []byte(""), Expected: []byte("ab")}}, false},
		{"test added", CheckerTokens, append([]Test{}, base[0], Test{Name: "02"}), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Version(tc.checker, tc.tests) == v; got != tc.wantEqual {
				t.Errorf("version equal = %v, want %v", got, tc.wantEqual)
			}
		})
	}
}

// A broad ignore rule such as *.out once hid every expected-output file from
// git, so a fresh clone could not load the sample problem even though all tests
// passed on the machine that had the files. No problem test file may be ignored.
func TestProblemTestFilesAreNotGitIgnored(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := filepath.Join("..", "..", "problems")
	files, err := filepath.Glob(filepath.Join(root, "*", "tests", "*"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no problem test files found under %s (err %v)", root, err)
	}
	for _, f := range files {
		cmd := exec.CommandContext(t.Context(), "git", "check-ignore", "-q", f) //nolint:gosec // fixed command, paths from our own glob
		err := cmd.Run()
		var exit *exec.ExitError
		switch {
		case err == nil:
			t.Errorf("%s is ignored by git, so it would be missing from a fresh clone", f)
		case errors.As(err, &exit) && exit.ExitCode() == 1:
			// not ignored: good
		default:
			t.Skipf("git check-ignore is not usable here: %v", err)
		}
	}
}

// TestSampleSumLoads checks the committed sample problem stays valid.
func TestSampleSumLoads(t *testing.T) {
	p, err := Load(filepath.Join("..", "..", "problems", "sample-sum"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tests) != 5 {
		t.Errorf("tests = %d, want 5", len(p.Tests))
	}
}
