// Command rejudge syncs one problem from the problems directory and rejudges
// its submissions that were judged against an older test-set version. The API
// does the same for every problem at startup; this is for doing it on demand
// (after fixing tests without restarting the API, or to retry rows a failed
// enqueue left behind). It holds the database credentials, as the API does;
// runners never do.
//
//	rejudge [-dry-run] [-batch N] <slug>
//
// Environment: DATABASE_URL, LEETFORCE_REDIS_URL (required), LEETFORCE_PROBLEMS_DIR
// (default "problems"), LEETFORCE_QUEUE_PREFIX, and LEETFORCE_S3_* when problem
// bundles live in object storage: the new bundle is published before any job
// names its version. Old bundles are never deleted.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"leetforce/api/internal/catalog"
	"leetforce/api/internal/rejudge"
	"leetforce/api/internal/store"
	"leetforce/judge/problem"
	"leetforce/queue"
	"leetforce/storage"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "rejudge:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("rejudge", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "only count the submissions that would be rejudged")
	batch := fs.Int("batch", rejudge.DefaultBatch, "submissions per transaction")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: rejudge [-dry-run] [-batch N] <problem-slug>")
	}
	slug := fs.Arg(0)

	dbURL, redisURL := os.Getenv("DATABASE_URL"), os.Getenv("LEETFORCE_REDIS_URL")
	if dbURL == "" || redisURL == "" {
		return errors.New("DATABASE_URL and LEETFORCE_REDIS_URL must be set")
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	dir := os.Getenv("LEETFORCE_PROBLEMS_DIR")
	if dir == "" {
		dir = "problems"
	}
	cat, err := catalog.Load(dir)
	if err != nil {
		return err
	}
	var found bool
	for _, p := range cat.Problems() {
		found = found || p.Spec.Slug == slug
	}
	if !found {
		return fmt.Errorf("no problem %q in %s", slug, dir)
	}

	db, err := store.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if *dry {
		n, err := db.PendingRejudge(ctx, slug)
		if err != nil {
			return err
		}
		fmt.Printf("%s: %d submissions are stale against the stored version (run without -dry-run to sync %s from %s and rejudge)\n", slug, n, slug, dir)
		return nil
	}

	// Publish the bundle before any job names its version.
	if s3cfg, useS3, err := storage.ConfigFromEnv(); err != nil {
		return err
	} else if useS3 {
		st, err := storage.Open(s3cfg)
		if err != nil {
			return err
		}
		if err := st.EnsureBucket(ctx); err != nil {
			return fmt.Errorf("object storage: %w", err)
		}
		if _, err := cat.Publish(ctx, st); err != nil {
			return fmt.Errorf("object storage: %w", err)
		}
	}

	var one []*problem.Problem
	for _, p := range cat.Problems() {
		if p.Spec.Slug == slug {
			one = append(one, p)
		}
	}
	changes, err := rejudge.Sync(ctx, db, one)
	if err != nil {
		return err
	}
	ch := changes[0]
	log.Info("problem synced", "problem", slug, "created", ch.Created, "old_version", ch.Old, "new_version", ch.New)

	q, err := queue.Open(redisURL, queue.Config{Prefix: os.Getenv("LEETFORCE_QUEUE_PREFIX")})
	if err != nil {
		return err
	}
	defer func() { _ = q.Close() }()
	if err := q.Ping(ctx); err != nil {
		return fmt.Errorf("connect to redis: %w", err)
	}
	if err := q.Setup(ctx); err != nil {
		return err
	}

	// Run (not RunChanged): also retries rows left stale by an earlier failure.
	res, err := rejudge.New(db, q, log, *batch).Run(ctx, slug)
	fmt.Printf("%s: requeued %d, deferred to the reaper %d\n", slug, res.Requeued, res.Deferred)
	return err
}
