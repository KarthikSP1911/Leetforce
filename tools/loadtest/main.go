// Command loadtest drives a LeetForce API with virtual users: sign-up, problem
// list, Run, Submit and the SSE stream until a verdict. It uses only the
// standard library (ADR 0024).
//
// Test accounts are named lfload_<runid>_<n>; see docs/phases/phase-16-log.md
// for the SQL that deletes them.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"
)

func main() {
	cfg := config{}
	base := os.Getenv("LEETFORCE_LOADTEST_BASE_URL")
	if base == "" {
		base = "http://localhost:8080"
	}
	flag.StringVar(&cfg.BaseURL, "base-url", base, "API base URL (env LEETFORCE_LOADTEST_BASE_URL)")
	flag.IntVar(&cfg.Users, "users", 5, "concurrent virtual users")
	flag.IntVar(&cfg.Iterations, "iterations", 0, "iterations per user (default 1 when -duration is 0)")
	flag.DurationVar(&cfg.Duration, "duration", 0, "stop starting iterations after this long (0: use -iterations)")
	flag.DurationVar(&cfg.Ramp, "ramp", 0, "spread virtual user start over this period")
	flag.StringVar(&cfg.Mode, "mode", "mixed", "mixed (problems, run, submit) or contest (submit to a contest)")
	flag.StringVar(&cfg.Contest, "contest", "", "contest slug for -mode contest")
	flag.StringVar(&cfg.ContestPath, "contest-path", "/contests/%s/submissions", "contest submit path, %s is the slug")
	flag.StringVar(&cfg.Problem, "problem", "", "problem slug (default: first in the list)")
	flag.StringVar(&cfg.Language, "language", "python", "python, cpp, java or go")
	flag.StringVar(&cfg.SourceFile, "source-file", "", "file with the solution to send (default: a trivial program)")
	flag.StringVar(&cfg.Password, "password", "loadtest-Passw0rd", "password for the test accounts")
	flag.DurationVar(&cfg.VerdictTimeout, "verdict-timeout", 2*time.Minute, "give up waiting for one verdict after this long")
	flag.StringVar(&cfg.JSONFile, "json", "", "also write the report as JSON to this file")
	flag.Parse()

	if cfg.SourceFile != "" {
		b, err := os.ReadFile(cfg.SourceFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "loadtest:", err)
			os.Exit(2)
		}
		cfg.Source = string(b)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	rep, err := run(ctx, cfg)
	if rep != nil {
		rep.WriteText(os.Stdout)
		if cfg.JSONFile != "" {
			if jerr := rep.WriteJSON(cfg.JSONFile); jerr != nil {
				fmt.Fprintln(os.Stderr, "loadtest:", jerr)
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "loadtest:", err)
		os.Exit(1)
	}
}
