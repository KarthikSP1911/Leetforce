//go:build linux

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"leetforce/judge/engine"
	"leetforce/judge/problem"
	"leetforce/judge/sandbox"
	"leetforce/judge/verdict"
)

const sampleDir = "../../../problems/sample-sum"

func TestLanguageFor(t *testing.T) {
	tests := []struct {
		file    string
		want    string
		wantErr bool
	}{
		{"a.py", "python", false},
		{"dir/ac.cpp", "cpp", false},
		{"x.CC", "cpp", false},
		{"Main.java", "java", false},
		{"sol.go", "go", false},
		{"notes.txt", "", true},
		{"noextension", "", true},
	}
	for _, tc := range tests {
		got, err := languageFor(tc.file)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("languageFor(%q) = %q, %v; want %q, error %v", tc.file, got, err, tc.want, tc.wantErr)
		}
	}
}

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRunUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no arguments", nil, "usage:"},
		{"unknown command", []string{"build"}, "usage:"},
		{"missing files", []string{"run"}, "usage:"},
		{"one argument", []string{"run", sampleDir}, "usage:"},
		{"bad flag", []string{"run", "-nope", sampleDir, "x.py"}, "flag provided but not defined"},
		{"unknown extension", []string{"run", sampleDir, "x.txt"}, "cannot tell the language"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runCLI(t, tc.args...)
			if code != 2 || !strings.Contains(stderr, tc.want) {
				t.Errorf("exit %d, stderr %q; want exit 2 containing %q", code, stderr, tc.want)
			}
		})
	}
}

func TestRunRequiresRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}
	code, _, stderr := runCLI(t, "run", sampleDir, "x.py")
	if code != 2 || !strings.Contains(stderr, "must run as root") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func loadSample(t *testing.T) *problem.Problem {
	t.Helper()
	p, err := problem.Load(sampleDir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPrintReport(t *testing.T) {
	p := loadSample(t)
	ms := time.Millisecond
	rep := &engine.Report{
		Slug: "sample-sum", Language: "python", TestSetVersion: "ts-abc",
		Cases: []engine.CaseResult{
			{Case: verdict.Case{Name: "01", Verdict: verdict.AC, Time: 12 * ms, Memory: 5 << 20}},
			{Case: verdict.Case{Name: "03", Verdict: verdict.WA, Time: 30 * ms, Memory: 7 << 20}},
		},
	}
	rep.Overall = verdict.Summarize(caseList(rep.Cases))
	var out bytes.Buffer
	printReport(&out, rep, p)
	got := out.String()
	for _, want := range []string{
		"Problem   sample-sum (test set ts-abc)", "Verdict   WA", "Runtime   30 ms", "Memory    7.0 MiB", "Failed    test 03",
		"01", "sample", "AC", "03", "hidden", "WA",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "---") {
		t.Errorf("no details were recorded, none should be printed:\n%s", got)
	}

	rep.Cases[0].Detail = &engine.Detail{Input: "1\n", Expected: "2\n", Actual: "3\n", Stderr: "boom"}
	out.Reset()
	printReport(&out, rep, p)
	for _, want := range []string{"--- test 01", "expected:\n2", "actual:\n3", "stderr:\nboom"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("detail missing %q:\n%s", want, out.String())
		}
	}
}

func TestPrintReportCompileError(t *testing.T) {
	p := loadSample(t)
	rep := &engine.Report{
		Slug: "sample-sum", Language: "go", TestSetVersion: "ts-abc",
		CompileOutput: "main.go:4:2: oops",
		Cases:         []engine.CaseResult{{Case: verdict.Case{Name: "compile", Verdict: verdict.CE}}},
	}
	rep.Overall = verdict.Summarize(caseList(rep.Cases))
	var out bytes.Buffer
	printReport(&out, rep, p)
	got := out.String()
	if !strings.Contains(got, "Verdict   CE") || !strings.Contains(got, "main.go:4:2: oops") {
		t.Errorf("CE report:\n%s", got)
	}
	if strings.Contains(got, "TEST") || strings.Contains(got, "Runtime") || strings.Contains(got, "Failed") {
		t.Errorf("CE report should have no test table, runtime or failed line:\n%s", got)
	}
}

func caseList(rs []engine.CaseResult) []verdict.Case {
	out := make([]verdict.Case, len(rs))
	for i, r := range rs {
		out[i] = r.Case
	}
	return out
}

// requireSandbox skips unless real programs can run in nsjail. It also caps all
// run cgroups at 450 MiB with no swap, as the engine tests do.
func requireSandbox(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root (run via make test-sandbox)")
	}
	if _, err := exec.LookPath("nsjail"); err != nil {
		t.Skip("nsjail not installed")
	}
	if err := os.MkdirAll(sandbox.DefaultCgroupRoot, 0o750); err != nil {
		t.Fatalf("create cgroup root: %v", err)
	}
	for file, value := range map[string]string{
		"cgroup.subtree_control": "+memory +pids +cpu",
		"memory.max":             "450M",
		"memory.swap.max":        "0",
	} {
		if err := os.WriteFile(filepath.Join(sandbox.DefaultCgroupRoot, file), []byte(value), 0o600); err != nil {
			t.Fatalf("set %s: %v", file, err)
		}
	}
}

func TestRunEndToEnd(t *testing.T) {
	requireSandbox(t)
	sol := func(name string) string { return filepath.Join(sampleDir, "solutions", "python", name) }

	code, stdout, stderr := runCLI(t, "run", sampleDir, sol("ac.py"))
	if code != 0 || !strings.Contains(stdout, "Verdict   AC") || stderr != "" {
		t.Fatalf("ac: exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	code, stdout, _ = runCLI(t, "run", sampleDir, sol("wa.py"))
	if code != 1 || !strings.Contains(stdout, "Verdict   WA") || !strings.Contains(stdout, "Failed    test 03") {
		t.Fatalf("wa: exit %d\n%s", code, stdout)
	}
	if strings.Contains(stdout, "\n04") {
		t.Errorf("without -all the run must stop at the first failure:\n%s", stdout)
	}

	code, stdout, _ = runCLI(t, "run", "-all", sampleDir, sol("wa.py"))
	if code != 1 || !strings.Contains(stdout, "\n05") {
		t.Fatalf("wa -all: exit %d\n%s", code, stdout)
	}

	code, stdout, _ = runCLI(t, "run", "-detail", sampleDir, sol("re.py"))
	if code != 1 || !strings.Contains(stdout, "Verdict   RE") || !strings.Contains(stdout, "ZeroDivisionError") {
		t.Fatalf("re -detail: exit %d\n%s", code, stdout)
	}

	code, stdout, _ = runCLI(t, "run", sampleDir, sol("ce.py"))
	if code != 1 || !strings.Contains(stdout, "Verdict   CE") || !strings.Contains(stdout, "SyntaxError") {
		t.Fatalf("ce: exit %d\n%s", code, stdout)
	}

	// An explicit -lang overrides the extension.
	code, _, stderr = runCLI(t, "run", "-lang", "cobol", sampleDir, sol("ac.py"))
	if code != 2 || !strings.Contains(stderr, "unknown language") {
		t.Errorf("-lang cobol: exit %d, stderr %q", code, stderr)
	}

	// A bad problem directory is a host/usage error, not a verdict.
	code, _, stderr = runCLI(t, "run", filepath.Join(t.TempDir(), "nope"), sol("ac.py"))
	if code != 2 || !strings.Contains(stderr, "load problem") {
		t.Errorf("missing problem: exit %d, stderr %q", code, stderr)
	}
}
