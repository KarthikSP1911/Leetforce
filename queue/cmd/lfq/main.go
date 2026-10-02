// Command lfq is a small tool for the job queue, for demos and tests:
//
//	lfq enqueue [-id ID] <problem-slug> <language> <source-file>
//	lfq results
//	lfq destroy      (only for a throwaway prefix, never the default one)
//
// It reads LEETFORCE_REDIS_URL and LEETFORCE_QUEUE_PREFIX from the environment.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"leetforce/queue"
)

const usage = `usage:
  lfq enqueue [-id ID] <problem-slug> <language> <source-file>
  lfq results
  lfq destroy`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lfq:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s", usage)
	}
	url := os.Getenv("LEETFORCE_REDIS_URL")
	if url == "" {
		return fmt.Errorf("LEETFORCE_REDIS_URL is not set")
	}
	prefix := os.Getenv("LEETFORCE_QUEUE_PREFIX")
	q, err := queue.Open(url, queue.Config{Prefix: prefix})
	if err != nil {
		return err
	}
	defer func() { _ = q.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch args[0] {
	case "enqueue":
		return enqueue(ctx, q, args[1:])
	case "results":
		rs, err := q.Results(ctx, "-")
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		for _, r := range rs {
			if err := enc.Encode(r); err != nil {
				return err
			}
		}
		return nil
	case "destroy":
		if prefix == "" || prefix == "leetforce" {
			return fmt.Errorf("refusing to destroy the default prefix; set LEETFORCE_QUEUE_PREFIX to a throwaway value")
		}
		return q.Destroy(ctx)
	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
}

func enqueue(ctx context.Context, q *queue.Queue, args []string) error {
	fs := flag.NewFlagSet("enqueue", flag.ContinueOnError)
	id := fs.String("id", "", "submission ID (default: random)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 3 {
		return fmt.Errorf("%s", usage)
	}
	src, err := os.ReadFile(fs.Arg(2))
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	if *id == "" {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return fmt.Errorf("random id: %w", err)
		}
		*id = "sub-" + hex.EncodeToString(b[:])
	}
	if err := q.Setup(ctx); err != nil {
		return err
	}
	entry, err := q.Enqueue(ctx, queue.Job{SubmissionID: *id, Problem: fs.Arg(0), Language: fs.Arg(1), Source: string(src)})
	if err != nil {
		return err
	}
	fmt.Printf("enqueued %s (entry %s)\n", *id, entry)
	return nil
}
