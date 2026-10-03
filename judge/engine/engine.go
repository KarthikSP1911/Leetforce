// Package engine judges one submission against one problem: it stores the
// source, compiles it in a sandbox, runs every test in a sandbox, and combines
// the host-measured facts and the output check into a verdict.
//
// Untrusted code only ever runs through sandbox.Run. Verdicts come from
// verdict.Classify (host measurements) and checker.Check (host-side comparison
// of captured stdout); nothing the program writes about itself is trusted.
package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"leetforce/judge/checker"
	"leetforce/judge/lang"
	"leetforce/judge/problem"
	"leetforce/judge/sandbox"
	"leetforce/judge/verdict"
)

const (
	defaultWorkRoot = "/var/tmp"
	// MaxSourceBytes is the largest source file the engine accepts.
	MaxSourceBytes = 64 << 10
	// maxCompileOutput is how much compiler output is kept for a compile error.
	maxCompileOutput = 4 << 10
	// wallSlack is added on top of twice the time limit before a run is killed
	// for wall time, so a sleeping program cannot hold a runner for long.
	wallSlack = time.Second
	// cpuKillSlack is added to the CPU limit before the kernel kills the
	// program. Classification uses the measured CPU time against the exact
	// limit, so a runaway always measures over it; a kill exactly at the limit
	// could measure just under it and look like a crash.
	cpuKillSlack = time.Second
	// detailBytes caps each field of a Detail.
	detailBytes = 4 << 10
	// MaxInputBytes is the largest custom input RunCustom accepts.
	MaxInputBytes = 8 << 10
	// customOutputBytes caps the stdout a custom run may produce.
	customOutputBytes = 64 << 10
)

var (
	// ErrUnknownLanguage is returned for a language the engine does not support.
	ErrUnknownLanguage = errors.New("unknown language")
	// ErrSourceTooLarge is returned when the source exceeds MaxSourceBytes.
	ErrSourceTooLarge = errors.New("source too large")
	// ErrInputTooLarge is returned when a custom input exceeds MaxInputBytes.
	ErrInputTooLarge = errors.New("input too large")
)

// Engine runs judging jobs. The zero value works on a Linux host as root.
type Engine struct {
	// WorkRoot is where job directories are created; empty means /var/tmp.
	// It must not be under /tmp: every sandbox mounts its own tmpfs there, which
	// would hide the bind-mounted job directory.
	WorkRoot string
	// NsjailPath and CgroupRoot override the sandbox defaults (used by tests).
	NsjailPath string
	CgroupRoot string
}

// Options change what a judging job does.
type Options struct {
	// ContinueOnFail runs every test even after a failure (for local tooling).
	// By default the job stops at the first failing test.
	ContinueOnFail bool
	// Detail records input, expected and actual output and stderr for failing
	// sample tests. It is for Run only. Hidden tests never get a Detail, and
	// Submit must not set it.
	Detail bool
}

// Detail is what Run may show about a failing sample test.
type Detail struct {
	Input    string
	Expected string
	Actual   string
	Stderr   string
}

// CaseResult is the outcome of one test.
type CaseResult struct {
	verdict.Case
	Detail *Detail
}

// CustomReport is the outcome of running a program once on a user-supplied
// input. There is no expected output, so Verdict is verdict.Completed ("")
// when the program ran cleanly within its limits. It is for Run only.
type CustomReport struct {
	Verdict       verdict.Verdict
	CompileOutput string
	Stdout        string
	Stderr        string
	Time          time.Duration
	Memory        uint64
}

// Report is the outcome of judging a submission.
type Report struct {
	Slug           string
	Language       string
	TestSetVersion string
	Overall        verdict.Overall
	Cases          []CaseResult
	// CompileOutput is the compiler's message when the verdict is CE.
	CompileOutput string
}

// Judge judges source, written in the named language, against the problem.
// An error means the host failed (sandbox setup, disk), not that the program
// is wrong; callers should retry such jobs rather than record a verdict.
func (e *Engine) Judge(ctx context.Context, p *problem.Problem, language string, source []byte, opts Options) (*Report, error) {
	lg, ok := lang.Get(language)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownLanguage, language)
	}
	if len(source) > MaxSourceBytes {
		return nil, fmt.Errorf("%w: %d bytes, limit %d", ErrSourceTooLarge, len(source), MaxSourceBytes)
	}

	dir, err := e.newJobDir(source, lg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	rep := &Report{Slug: p.Spec.Slug, Language: language, TestSetVersion: p.TestSetVer}

	if lg.Compile != nil {
		out, ok, err := e.compile(ctx, dir, lg)
		if err != nil {
			return nil, err
		}
		if !ok {
			rep.CompileOutput = out
			rep.Cases = []CaseResult{{Case: verdict.Case{Name: "compile", Verdict: verdict.CE}}}
			rep.Overall = verdict.Summarize(caseList(rep.Cases))
			return rep, nil
		}
	}

	limit := p.Spec.LimitFor(language)
	vlimits := verdict.Limits{Time: limit.Time(), MemoryBytes: limit.MemoryBytes(), OOMExitCode: lg.OOMExitCode}
	for _, t := range p.Tests {
		cr, err := e.runTest(ctx, dir, lg, p.Spec.Checker, limit, vlimits, t, opts)
		if err != nil {
			return nil, err
		}
		rep.Cases = append(rep.Cases, cr)
		if cr.Verdict != verdict.AC && !opts.ContinueOnFail {
			break
		}
	}
	rep.Overall = verdict.Summarize(caseList(rep.Cases))
	return rep, nil
}

// RunCustom compiles source and runs it once on input under the problem's
// limits for the language. Like Judge, an error means the host failed, except
// for ErrUnknownLanguage, ErrSourceTooLarge and ErrInputTooLarge, which say the
// request itself was bad.
func (e *Engine) RunCustom(ctx context.Context, p *problem.Problem, language string, source, input []byte) (*CustomReport, error) {
	lg, ok := lang.Get(language)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownLanguage, language)
	}
	if len(source) > MaxSourceBytes {
		return nil, fmt.Errorf("%w: %d bytes, limit %d", ErrSourceTooLarge, len(source), MaxSourceBytes)
	}
	if len(input) > MaxInputBytes {
		return nil, fmt.Errorf("%w: %d bytes, limit %d", ErrInputTooLarge, len(input), MaxInputBytes)
	}

	dir, err := e.newJobDir(source, lg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if lg.Compile != nil {
		out, ok, err := e.compile(ctx, dir, lg)
		if err != nil {
			return nil, err
		}
		if !ok {
			return &CustomReport{Verdict: verdict.CE, CompileOutput: out}, nil
		}
	}

	limit := p.Spec.LimitFor(language)
	vlimits := verdict.Limits{Time: limit.Time(), MemoryBytes: limit.MemoryBytes(), OOMExitCode: lg.OOMExitCode}
	res, err := e.runProgram(ctx, dir, lg, limit, input, customOutputBytes)
	if err != nil {
		return nil, fmt.Errorf("run custom input: %w", err)
	}
	return &CustomReport{
		Verdict: verdict.Classify(*res, vlimits),
		Stdout:  truncate(strings.ToValidUTF8(string(res.Stdout), "�"), detailBytes),
		Stderr:  truncate(strings.ToValidUTF8(string(res.Stderr), "�"), detailBytes),
		Time:    res.CPUTime,
		Memory:  res.PeakMemoryBytes,
	}, nil
}

func caseList(rs []CaseResult) []verdict.Case {
	out := make([]verdict.Case, len(rs))
	for i, r := range rs {
		out[i] = r.Case
	}
	return out
}

// newJobDir creates the job directory with the source in it. Directories and
// files are world-readable because the program runs as an unprivileged user.
func (e *Engine) newJobDir(source []byte, lg lang.Language) (string, error) {
	root := e.WorkRoot
	if root == "" {
		root = defaultWorkRoot
	}
	if root = filepath.Clean(root); root == "/tmp" || strings.HasPrefix(root, "/tmp/") {
		return "", fmt.Errorf("work root %q must not be under /tmp", root)
	}
	dir, err := os.MkdirTemp(root, "leetforce-job-")
	if err != nil {
		return "", fmt.Errorf("create job dir: %w", err)
	}
	fail := func(err error) (string, error) {
		_ = os.RemoveAll(dir)
		return "", err
	}
	for _, d := range []string{dir, filepath.Join(dir, "src"), filepath.Join(dir, "bin")} {
		if err := os.MkdirAll(d, 0o755); err != nil { //nolint:gosec // must be traversable by the sandbox user
			return fail(fmt.Errorf("create %s: %w", d, err))
		}
		if err := os.Chmod(d, 0o755); err != nil { //nolint:gosec // MkdirTemp makes 0700; the sandbox user must traverse it
			return fail(fmt.Errorf("chmod %s: %w", d, err))
		}
	}
	if err := os.WriteFile(lg.SourcePath(dir), source, 0o644); err != nil { //nolint:gosec // readable by the sandbox user
		return fail(fmt.Errorf("write source: %w", err))
	}
	return dir, nil
}

// compile runs the language's compile step in a sandbox. It returns the
// compiler output and whether the build succeeded; the error is for host
// failures only.
func (e *Engine) compile(ctx context.Context, dir string, lg lang.Language) (output string, ok bool, err error) {
	cl := lg.CompileLimits
	limits := sandbox.DefaultLimits()
	limits.WallTime = cl.Time
	limits.CPUTime = cl.Time
	limits.MemoryBytes = cl.MemoryBytes
	limits.MaxPIDs = cl.PIDs
	limits.TmpfsBytes = cl.TmpfsBytes
	if cl.MaxFileBytes > 0 {
		limits.MaxFileBytes = cl.MaxFileBytes
	}
	limits.MaxOutputBytes = maxCompileOutput
	limits.MaxResultBytes = cl.MaxArtifactBytes

	res, err := sandbox.Run(ctx, sandbox.Spec{
		Argv:          lg.Compile(dir),
		Env:           lg.CompileEnv,
		ReadOnlyBinds: append([]string{filepath.Join(dir, "src")}, lg.Binds...),
		Limits:        limits,
		NsjailPath:    e.NsjailPath,
		CgroupRoot:    e.CgroupRoot,
	})
	if err != nil {
		return "", false, fmt.Errorf("compile %s: %w", lg.Name, err)
	}
	output = cleanOutput(string(res.Stderr)+string(res.Stdout), dir)

	clean := res.ExitCode == 0 && res.Signal == 0 && !res.TimedOut && !res.OutputExceeded && !res.OOMKilled
	if !clean {
		if res.TimedOut {
			output += "\ncompile time limit exceeded"
		}
		if res.OOMKilled {
			output += "\ncompile memory limit exceeded"
		}
		return output, false, nil
	}
	if lg.Artifact {
		if len(res.ResultData) == 0 {
			return output + "\ncompiler produced no output", false, nil
		}
		if err := os.WriteFile(lang.ArtifactPath(dir), res.ResultData, 0o755); err != nil { //nolint:gosec // must be executable by the sandbox user
			return "", false, fmt.Errorf("store artifact: %w", err)
		}
	}
	return output, true, nil
}

// cleanOutput makes compiler output safe to show: no host paths, valid UTF-8,
// and a bounded length.
func cleanOutput(s, dir string) string {
	s = strings.ReplaceAll(s, filepath.Join(dir, "src")+string(filepath.Separator), "")
	s = strings.ToValidUTF8(s, "�")
	return truncate(strings.TrimSpace(s), maxCompileOutput)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

func (e *Engine) runTest(ctx context.Context, dir string, lg lang.Language, mode string, limit problem.Limit, vl verdict.Limits, t problem.Test, opts Options) (CaseResult, error) {
	res, err := e.runProgram(ctx, dir, lg, limit, t.Input, outputCap(len(t.Expected)))
	if err != nil {
		return CaseResult{}, fmt.Errorf("run test %s: %w", t.Name, err)
	}

	v := verdict.Classify(*res, vl)
	if v == verdict.Completed {
		v, err = checker.Check(mode, t.Expected, res.Stdout)
		if err != nil {
			return CaseResult{}, fmt.Errorf("check test %s: %w", t.Name, err)
		}
	}
	cr := CaseResult{Case: verdict.Case{Name: t.Name, Verdict: v, Time: res.CPUTime, Memory: res.PeakMemoryBytes}}
	if opts.Detail && t.Sample && v != verdict.AC {
		cr.Detail = &Detail{
			Input:    truncate(strings.ToValidUTF8(string(t.Input), "�"), detailBytes),
			Expected: truncate(strings.ToValidUTF8(string(t.Expected), "�"), detailBytes),
			Actual:   truncate(strings.ToValidUTF8(string(res.Stdout), "�"), detailBytes),
			Stderr:   truncate(strings.ToValidUTF8(string(res.Stderr), "�"), detailBytes),
		}
	}
	return cr, nil
}

// runProgram runs the compiled program once in a sandbox with the given stdin
// and stdout cap, under the problem's limits for the language.
func (e *Engine) runProgram(ctx context.Context, dir string, lg lang.Language, limit problem.Limit, stdin []byte, maxOut int64) (*sandbox.Result, error) {
	limits := sandbox.DefaultLimits()
	limits.CPUTime = limit.Time() + cpuKillSlack
	limits.WallTime = 2*limit.Time() + wallSlack
	limits.MemoryBytes = limit.MemoryBytes()
	limits.MaxPIDs = lg.RunPIDs
	limits.MaxOutputBytes = maxOut
	limits.MaxResultBytes = 4 << 10
	limits.TmpfsBytes = 8 << 20

	return sandbox.Run(ctx, sandbox.Spec{
		Argv:          lg.Run(dir, limit.MemoryBytes()),
		Env:           lg.RunEnv,
		Stdin:         bytes.NewReader(stdin),
		ReadOnlyBinds: append([]string{dir}, lg.Binds...),
		Limits:        limits,
		NsjailPath:    e.NsjailPath,
		CgroupRoot:    e.CgroupRoot,
	})
}

// outputCap is the most stdout a test may produce: generous compared with the
// expected output so a wrong but plausible answer is WA, and small enough that
// a flood is stopped quickly.
func outputCap(expectedLen int) int64 {
	return max(int64(64<<10), 2*int64(expectedLen)+(4<<10))
}
