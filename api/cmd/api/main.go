// Command api is the LeetForce HTTP API. Configuration comes from the environment:
//
//	DATABASE_URL            required, Neon Postgres URL
//	LEETFORCE_REDIS_URL     required, redis:// or rediss:// URL
//	LEETFORCE_API_ADDR      listen address (default ":8080")
//	LEETFORCE_PROBLEMS_DIR  problem directory (default "problems")
//	LEETFORCE_QUEUE_PREFIX  queue key prefix (default "leetforce")
//	LEETFORCE_S3_ENDPOINT   object storage host:port; when set, problem bundles are published
//	                        to the bucket at startup (LEETFORCE_S3_ACCESS_KEY, _SECRET_KEY, _BUCKET, _USE_TLS)
//	LEETFORCE_REAPER_INTERVAL  how often stored-but-never-queued submissions are re-queued (default 15m;
//	                        it also sweeps once at startup. Each sweep wakes Neon, so keep it long)
//	LEETFORCE_REAPER_GRACE  how old such a submission must be before it is re-queued (default 2m)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"leetforce/api/internal/catalog"
	"leetforce/api/internal/ingest"
	"leetforce/api/internal/reaper"
	"leetforce/api/internal/server"
	"leetforce/api/internal/store"
	"leetforce/queue"
	"leetforce/storage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
}

func run() error {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	dbURL := os.Getenv("DATABASE_URL")
	redisURL := os.Getenv("LEETFORCE_REDIS_URL")
	if dbURL == "" || redisURL == "" {
		return errors.New("DATABASE_URL and LEETFORCE_REDIS_URL must be set")
	}
	addr := os.Getenv("LEETFORCE_API_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	problemsDir := os.Getenv("LEETFORCE_PROBLEMS_DIR")
	if problemsDir == "" {
		problemsDir = "problems"
	}
	cat, err := catalog.Load(problemsDir)
	if err != nil {
		return err
	}
	for _, p := range cat.Problems() {
		sp := store.Problem{Slug: p.Spec.Slug, Title: p.Spec.Title, Difficulty: p.Spec.Difficulty, Tags: p.Spec.Tags}
		if err := db.UpsertProblem(ctx, sp, p.TestSetVer); err != nil {
			return err
		}
	}
	log.Info("problems synced", "count", len(cat.Problems()))

	ready := map[string]server.Pinger{"database": db}
	s3cfg, useS3, err := storage.ConfigFromEnv()
	if err != nil {
		return err
	}
	if useS3 {
		st, err := storage.Open(s3cfg)
		if err != nil {
			return err
		}
		if err := st.EnsureBucket(ctx); err != nil {
			return fmt.Errorf("object storage: %w", err)
		}
		// Publish before accepting submissions, so every version a submission
		// can be stamped with is already in the bucket.
		n, err := cat.Publish(ctx, st)
		if err != nil {
			return fmt.Errorf("object storage: %w", err)
		}
		log.Info("problem bundles published", "bucket", s3cfg.Bucket, "written", n, "total", len(cat.Problems()))
		ready["storage"] = st
	}

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
	if err := q.SetupAPI(ctx); err != nil {
		return err
	}
	ready["redis"] = q

	host, _ := os.Hostname()
	ing := ingest.New(q, db, log, ingest.Config{Consumer: fmt.Sprintf("api-%s-%d", host, os.Getpid())})
	ing.SetRuns(q)
	ingestDone := make(chan struct{})
	go func() { ing.Run(ctx); close(ingestDone) }()
	watcher := ingest.NewStatusWatcher(q, db, log, ingest.StatusConfig{})
	statusDone := make(chan struct{})
	go func() { watcher.Run(ctx); close(statusDone) }()

	every, err := envDuration("LEETFORCE_REAPER_INTERVAL")
	if err != nil {
		return err
	}
	grace, err := envDuration("LEETFORCE_REAPER_GRACE")
	if err != nil {
		return err
	}
	rp := reaper.New(db, q, log, reaper.Config{Every: every, Grace: grace})
	reaperDone := make(chan struct{})
	go func() { rp.Run(ctx); close(reaperDone) }()

	srv := &http.Server{
		Addr:              addr,
		Handler:           server.New(server.Deps{Logger: log, Ready: ready, Problems: db, Samples: cat, Content: cat, Submissions: db, Queue: q, Versions: db, Runs: q}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("api listening", "addr", addr)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	stop()
	<-ingestDone
	<-statusDone
	<-reaperDone
	log.Info("api stopped")
	return nil
}

// envDuration reads a positive duration from the environment; unset means 0,
// which the callee replaces with its default.
func envDuration(name string) (time.Duration, error) {
	v := os.Getenv(name)
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s: %q is not a positive duration", name, v)
	}
	return d, nil
}
