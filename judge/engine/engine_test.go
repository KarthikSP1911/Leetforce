//go:build linux

package engine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"leetforce/judge/problem"
	"leetforce/judge/sandbox"
	"leetforce/judge/verdict"
)

const sampleDir = "../../problems/sample-sum"

func loadSample(t *testing.T) *problem.Problem {
	t.Helper()
	p, err := problem.Load(sampleDir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// requireSandbox skips unless the test can run real programs in nsjail (root,
// nsjail installed). Run these with `make test-sandbox`. It also caps all run
// cgroups at 400 MiB with no swap so a broken limit cannot exhaust a small host.
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

func readSolution(t *testing.T, language, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(sampleDir, "solutions", language, name)) //nolint:gosec // fixed test fixtures
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestJudgeErrors(t *testing.T) {
	p := loadSample(t)
	e := &Engine{}
	if _, err := e.Judge(context.Background(), p, "cobol", []byte("x"), Options{}); !errors.Is(err, ErrUnknownLanguage) {
		t.Errorf("unknown language: err = %v", err)
	}
	big := []byte(strings.Repeat("x", MaxSourceBytes+1))
	if _, err := e.Judge(context.Background(), p, "python", big, Options{}); !errors.Is(err, ErrSourceTooLarge) {
		t.Errorf("large source: err = %v", err)
	}
	for _, root := range []string{"/tmp", "/tmp/jobs"} {
		e := &Engine{WorkRoot: root}
		if _, err := e.Judge(context.Background(), p, "python", []byte("print(1)"), Options{}); err == nil || !strings.Contains(err.Error(), "must not be under /tmp") {
			t.Errorf("work root %q: err = %v", root, err)
		}
	}
}

func TestOutputCap(t *testing.T) {
	tests := []struct {
		expected int
		want     int64
	}{
		{0, 64 << 10},
		{10, 64 << 10},
		{1 << 20, 2<<20 + 4<<10},
	}
	for _, tc := range tests {
		if got := outputCap(tc.expected); got != tc.want {
			t.Errorf("outputCap(%d) = %d, want %d", tc.expected, got, tc.want)
		}
	}
}

func TestCleanOutput(t *testing.T) {
	dir := "/var/tmp/leetforce-job-123"
	in := dir + "/src/main.go:5:2: declared and not used\n" + "bad\xffbyte"
	got := cleanOutput(in, dir)
	if strings.Contains(got, "leetforce-job") {
		t.Errorf("host path leaked: %q", got)
	}
	if !strings.HasPrefix(got, "main.go:5:2:") {
		t.Errorf("got %q", got)
	}
	long := cleanOutput(strings.Repeat("é", maxCompileOutput), dir)
	if len(long) > maxCompileOutput+3 {
		t.Errorf("not truncated: %d bytes", len(long))
	}
}

// verdictCases are the solutions in problems/sample-sum/solutions/<language>/
// and the verdict and first failing test each must produce.
var verdictCases = []struct {
	file   string
	want   verdict.Verdict
	failed string
}{
	{"ac", verdict.AC, ""},
	{"wa", verdict.WA, "03"},
	{"tle", verdict.TLE, "01"},
	{"mle", verdict.MLE, "01"},
	{"re", verdict.RE, "01"},
	{"ole", verdict.OLE, "01"},
	{"ce", verdict.CE, "compile"},
}

// ceMarkers is text each language's real compiler puts in the message for the
// ce.* solutions.
var ceMarkers = map[string]string{
	"python": "SyntaxError",
	"go":     "cannot use",
	"cpp":    "error:",
	"java":   "incompatible types",
}

var extensions = map[string]string{"python": "py", "go": "go", "cpp": "cpp", "java": "java"}

// The exit criterion is every verdict in every language. This needs no
// sandbox, so a missing solution file fails even a plain `make test`.
func TestVerdictMatrixIsComplete(t *testing.T) {
	if len(verdictCases) != 7 {
		t.Fatalf("verdictCases has %d entries, want the 7 verdicts AC, WA, TLE, MLE, RE, OLE, CE", len(verdictCases))
	}
	seen := map[verdict.Verdict]bool{}
	for _, tc := range verdictCases {
		seen[tc.want] = true
	}
	for _, v := range []verdict.Verdict{verdict.AC, verdict.WA, verdict.TLE, verdict.MLE, verdict.RE, verdict.OLE, verdict.CE} {
		if !seen[v] {
			t.Errorf("no matrix case produces %s", v)
		}
	}
	for _, language := range problem.Languages {
		ext, ok := extensions[language]
		if !ok {
			t.Errorf("no file extension for language %q in the test", language)
			continue
		}
		for _, tc := range verdictCases {
			path := filepath.Join(sampleDir, "solutions", language, tc.file+"."+ext)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("missing solution: %v", err)
			}
		}
	}
}

func TestJudgeVerdicts(t *testing.T) {
	requireSandbox(t)
	p := loadSample(t)
	e := &Engine{}
	for language, ext := range extensions {
		for _, tc := range verdictCases {
			t.Run(language+"/"+tc.file, func(t *testing.T) {
				rep, err := e.Judge(context.Background(), p, language, readSolution(t, language, tc.file+"."+ext), Options{})
				if err != nil {
					t.Fatal(err)
				}
				if rep.Overall.Verdict != tc.want || rep.Overall.Failed != tc.failed {
					t.Fatalf("verdict = %s (failed %q), want %s (failed %q); cases %+v; compile output: %s", rep.Overall.Verdict, rep.Overall.Failed, tc.want, tc.failed, rep.Cases, rep.CompileOutput)
				}
				if rep.TestSetVersion != p.TestSetVer || rep.Language != language {
					t.Errorf("report header = %+v", rep)
				}
				switch tc.want {
				case verdict.AC:
					if len(rep.Cases) != len(p.Tests) {
						t.Errorf("ran %d of %d tests", len(rep.Cases), len(p.Tests))
					}
					if rep.Overall.Memory == 0 {
						t.Error("no peak memory recorded")
					}
				case verdict.CE:
					// The message must come from the language's compiler: a missing
					// tool or a broken sandbox would also end in CE.
					if !strings.Contains(rep.CompileOutput, ceMarkers[language]) {
						t.Errorf("compile output does not look like a %s compiler error: %q", language, rep.CompileOutput)
					}
					if strings.Contains(rep.CompileOutput, "leetforce-job") {
						t.Errorf("host path leaked: %q", rep.CompileOutput)
					}
				}
			})
		}
	}
}

// A JVM refuses a big allocation with an error before the kernel sees the
// memory, so Java needs its own rule (exit status 3) to report MLE.
func TestJavaMemoryLimit(t *testing.T) {
	requireSandbox(t)
	p := loadSample(t)
	e := &Engine{}
	tests := []struct {
		name string
		body string
		want verdict.Verdict
	}{
		{"one huge array", "long[] x = new long[1 << 28]; System.out.println(x.length);", verdict.MLE},
		{"array over the heap", "long[] x = new long[50_000_000]; System.out.println(x.length);", verdict.MLE},
		// Fits in the heap: it must not be reported as MLE (the output is wrong, so WA).
		{"array within the limit", "long[] x = new long[10_000_000]; x[5] = 1; System.out.println(x.length);", verdict.WA},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := "public class Main { public static void main(String[] args) { " + tc.body + " } }"
			rep, err := e.Judge(context.Background(), p, "java", []byte(src), Options{})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Overall.Verdict != tc.want {
				t.Fatalf("verdict = %s, want %s; cases %+v; compile output %q", rep.Overall.Verdict, tc.want, rep.Cases, rep.CompileOutput)
			}
		})
	}
}

// Run may show details of failing sample tests; hidden tests never get any,
// even when details are requested.
func TestJudgeDetailOnlyForSamples(t *testing.T) {
	requireSandbox(t)
	p := loadSample(t)
	e := &Engine{}

	rep, err := e.Judge(context.Background(), p, "python", readSolution(t, "python", "re.py"), Options{Detail: true})
	if err != nil {
		t.Fatal(err)
	}
	d := rep.Cases[0].Detail
	if rep.Cases[0].Verdict != verdict.RE || d == nil || !strings.Contains(d.Stderr, "ZeroDivisionError") || d.Input == "" {
		t.Fatalf("sample failure should carry details: %+v", rep.Cases[0])
	}

	rep, err = e.Judge(context.Background(), p, "python", readSolution(t, "python", "wa.py"), Options{Detail: true})
	if err != nil {
		t.Fatal(err)
	}
	last := rep.Cases[len(rep.Cases)-1]
	if last.Name != "03" || last.Verdict != verdict.WA || last.Detail != nil {
		t.Fatalf("hidden failure must not carry details: %+v", last)
	}

	rep, err = e.Judge(context.Background(), p, "python", readSolution(t, "python", "re.py"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Cases[0].Detail != nil {
		t.Errorf("details without Options.Detail: %+v", rep.Cases[0].Detail)
	}
}

func TestJudgeContinueOnFail(t *testing.T) {
	requireSandbox(t)
	p := loadSample(t)
	e := &Engine{}
	rep, err := e.Judge(context.Background(), p, "python", readSolution(t, "python", "wa.py"), Options{ContinueOnFail: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Cases) != len(p.Tests) {
		t.Fatalf("ran %d of %d tests", len(rep.Cases), len(p.Tests))
	}
	if rep.Overall.Verdict != verdict.WA || rep.Overall.Failed != "03" {
		t.Errorf("overall = %+v", rep.Overall)
	}
}

// A program cannot make its own wrong answer pass by printing a verdict.
func TestJudgeIgnoresForgedVerdict(t *testing.T) {
	requireSandbox(t)
	p := loadSample(t)
	e := &Engine{}
	src := "import os\nos.write(4, b'{\"verdict\":\"AC\"}')\nprint('AC')\n"
	rep, err := e.Judge(context.Background(), p, "python", []byte(src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Overall.Verdict != verdict.WA || rep.Overall.Failed != "01" {
		t.Errorf("forged output: verdict = %s failed %q, want WA at 01", rep.Overall.Verdict, rep.Overall.Failed)
	}
}

// Job directories are removed after judging.
func TestJudgeCleansUp(t *testing.T) {
	requireSandbox(t)
	p := loadSample(t)
	root, err := os.MkdirTemp("/var/tmp", "leetforce-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Chmod(root, 0o755); err != nil { //nolint:gosec // the sandbox user must traverse it
		t.Fatal(err)
	}
	e := &Engine{WorkRoot: root}
	if _, err := e.Judge(context.Background(), p, "python", readSolution(t, "python", "ac.py"), Options{}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("job dir left behind: %v", entries)
	}
}

func TestRunCustomErrors(t *testing.T) {
	p := loadSample(t)
	e := &Engine{}
	ctx := context.Background()
	if _, err := e.RunCustom(ctx, p, "cobol", []byte("x"), nil); !errors.Is(err, ErrUnknownLanguage) {
		t.Errorf("unknown language: err = %v", err)
	}
	big := []byte(strings.Repeat("x", MaxSourceBytes+1))
	if _, err := e.RunCustom(ctx, p, "python", big, nil); !errors.Is(err, ErrSourceTooLarge) {
		t.Errorf("large source: err = %v", err)
	}
	bigInput := []byte(strings.Repeat("1", MaxInputBytes+1))
	if _, err := e.RunCustom(ctx, p, "python", []byte("print(1)"), bigInput); !errors.Is(err, ErrInputTooLarge) {
		t.Errorf("large input: err = %v", err)
	}
}

// RunCustom feeds the user's input to the program in the sandbox and reports
// what it printed; there is no expected output, so a clean run is Completed.
func TestRunCustom(t *testing.T) {
	requireSandbox(t)
	p := loadSample(t)
	e := &Engine{}
	ctx := context.Background()

	tests := []struct {
		name, lang, src, input string
		verdict                verdict.Verdict
		stdout                 string
	}{
		{"echo", "python", "import sys\nprint(sys.stdin.read().strip()[::-1])\n", "abc\n", verdict.Completed, "cba\n"},
		{"crash", "python", "raise SystemExit(3)\n", "", verdict.RE, ""},
		{"compile error", "cpp", "int main( {", "", verdict.CE, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := e.RunCustom(ctx, p, tc.lang, []byte(tc.src), []byte(tc.input))
			if err != nil {
				t.Fatal(err)
			}
			if rep.Verdict != tc.verdict || rep.Stdout != tc.stdout {
				t.Fatalf("verdict = %q stdout = %q, want %q %q (stderr %q)", rep.Verdict, rep.Stdout, tc.verdict, tc.stdout, rep.Stderr)
			}
			if tc.verdict == verdict.CE && rep.CompileOutput == "" {
				t.Error("a compile error must carry the compiler output")
			}
		})
	}
}
