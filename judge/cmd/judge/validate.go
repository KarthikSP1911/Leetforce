package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"leetforce/judge/engine"
	"leetforce/judge/problem"
	"leetforce/judge/validate"
)

// runValidate implements `judge validate`. Exit status: 0 all problems valid,
// 1 a problem failed validation, 2 usage or host error.
func runValidate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, usage) }
	structureOnly := fs.Bool("structure-only", false, "skip the reference-solution check (no sandbox needed)")
	strict := fs.Bool("strict", false, "treat warnings as failures")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	dirs, err := validate.Dirs(fs.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "judge: %v\n", err)
		return 2
	}
	if !*structureOnly && os.Geteuid() != 0 {
		_, _ = fmt.Fprintf(stderr, "judge: %v\n", validate.ErrNoSandbox)
		return 2
	}

	failed := 0
	for _, dir := range dirs {
		res := validate.Structure(dir)
		if !res.Failed(*strict) && !*structureOnly {
			p, err := problem.Load(dir)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "judge: %v\n", err)
				return 2
			}
			issues, err := validate.Reference(ctx, &engine.Engine{}, p)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "judge: %s: %v\n", dir, err)
				return 2
			}
			res.Issues = append(res.Issues, issues...)
		}
		printValidation(stdout, res, *strict)
		if res.Failed(*strict) {
			failed++
		}
	}
	_, _ = fmt.Fprintf(stdout, "\n%d of %d problems valid\n", len(dirs)-failed, len(dirs))
	if failed > 0 {
		return 1
	}
	return 0
}

func printValidation(w io.Writer, r *validate.Result, strict bool) {
	status := "ok"
	if r.Failed(strict) {
		status = "FAIL"
	}
	_, _ = fmt.Fprintf(w, "%-5s %s\n", status, r.Dir)
	for _, i := range r.Issues {
		_, _ = fmt.Fprintf(w, "      %s\n", i)
	}
}
