// Command judge judges a solution file against a problem directory on this
// machine. It is a local tool for authors and developers; it needs the same
// privileges as the sandbox (root, nsjail, cgroup v2).
//
//	judge run [-all] [-detail] [-lang NAME] problems/<slug> <solution-file>
//	judge validate [-structure-only] [-strict] problems/<slug>|problems
//
// The exit status is 0 for AC, 1 for any other verdict, and 2 for a usage or
// host error.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"leetforce/judge/engine"
	"leetforce/judge/problem"
	"leetforce/judge/verdict"
)

const usage = `usage: judge validate [-structure-only] [-strict] <problem-dir|problems-dir>
       judge run [-all] [-detail] [-lang NAME] <problem-dir> <solution-file>

  -all       run every test even after a failure
  -detail    show input, expected output, actual output and stderr for failing sample tests
  -lang      python, cpp, java or go (default: from the file extension)

Java solutions must declare "public class Main". Needs root, nsjail and cgroup v2.

validate checks problem.yaml, the statement, starters and test files, then judges
every solutions/<lang>/<verdict>.<ext> and requires that verdict (the second step
needs root like run; -structure-only skips it, -strict also fails on warnings).
`

// extensions maps file extensions to language names.
var extensions = map[string]string{
	".py":   "python",
	".cpp":  "cpp",
	".cc":   "cpp",
	".java": "java",
	".go":   "go",
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "validate" {
		return runValidate(ctx, args[1:], stdout, stderr)
	}
	if len(args) == 0 || args[0] != "run" {
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, usage) }
	all := fs.Bool("all", false, "run every test even after a failure")
	detail := fs.Bool("detail", false, "show details of failing sample tests")
	langName := fs.String("lang", "", "language (default: from the file extension)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return 2
	}
	dir, file := fs.Arg(0), fs.Arg(1)

	if *langName == "" {
		l, err := languageFor(file)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "judge: %v\n", err)
			return 2
		}
		*langName = l
	}
	if os.Geteuid() != 0 {
		_, _ = fmt.Fprintln(stderr, "judge: must run as root (the sandbox needs it); try: sudo -n bin/judge run ...")
		return 2
	}
	p, err := problem.Load(dir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "judge: %v\n", err)
		return 2
	}
	source, err := os.ReadFile(file) //nolint:gosec // the operator names the solution file
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "judge: read solution: %v\n", err)
		return 2
	}

	rep, err := (&engine.Engine{}).Judge(ctx, p, *langName, source, engine.Options{ContinueOnFail: *all, Detail: *detail})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "judge: %v\n", err)
		return 2
	}
	printReport(stdout, rep, p)
	if rep.Overall.Verdict == verdict.AC {
		return 0
	}
	return 1
}

// languageFor picks the language from a file name's extension.
func languageFor(file string) (string, error) {
	ext := strings.ToLower(filepath.Ext(file))
	if l, ok := extensions[ext]; ok {
		return l, nil
	}
	return "", fmt.Errorf("cannot tell the language of %q (extension %q); use -lang", file, ext)
}

func printReport(w io.Writer, rep *engine.Report, p *problem.Problem) {
	o := rep.Overall
	_, _ = fmt.Fprintf(w, "Problem   %s (test set %s)\n", rep.Slug, rep.TestSetVersion)
	_, _ = fmt.Fprintf(w, "Language  %s\n", rep.Language)
	_, _ = fmt.Fprintf(w, "Verdict   %s\n", o.Verdict)
	if o.Verdict != verdict.CE {
		_, _ = fmt.Fprintf(w, "Runtime   %s\n", formatTime(o.Time))
		_, _ = fmt.Fprintf(w, "Memory    %s\n", formatMemory(o.Memory))
	}
	if o.Failed != "" && o.Verdict != verdict.CE {
		_, _ = fmt.Fprintf(w, "Failed    test %s\n", o.Failed)
	}
	if rep.CompileOutput != "" {
		_, _ = fmt.Fprintf(w, "\nCompiler output:\n%s\n", rep.CompileOutput)
	}
	if o.Verdict == verdict.CE {
		return
	}

	_, _ = fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TEST\tKIND\tVERDICT\tTIME\tMEMORY")
	kind := make(map[string]string, len(p.Tests))
	for _, t := range p.Tests {
		kind[t.Name] = "hidden"
		if t.Sample {
			kind[t.Name] = "sample"
		}
	}
	for _, c := range rep.Cases {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", c.Name, kind[c.Name], c.Verdict, formatTime(c.Time), formatMemory(c.Memory))
	}
	_ = tw.Flush()

	for _, c := range rep.Cases {
		if c.Detail == nil {
			continue
		}
		_, _ = fmt.Fprintf(w, "\n--- test %s (%s) ---\n", c.Name, c.Verdict)
		_, _ = fmt.Fprintf(w, "input:\n%s\nexpected:\n%s\nactual:\n%s\n", c.Detail.Input, c.Detail.Expected, c.Detail.Actual)
		if c.Detail.Stderr != "" {
			_, _ = fmt.Fprintf(w, "stderr:\n%s\n", c.Detail.Stderr)
		}
	}
}

func formatTime(d time.Duration) string {
	return fmt.Sprintf("%d ms", d.Milliseconds())
}

func formatMemory(b uint64) string {
	return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20))
}
