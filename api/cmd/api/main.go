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
//	LEETFORCE_TRUSTED_PROXIES  comma-separated proxy addresses whose X-Forwarded-For is believed when
//	                        finding the client IP for rate limits (default none; set 127.0.0.1 behind the
//	                        local Next.js proxy)
//	LEETFORCE_LIMIT_SUBMIT_USER, _SUBMIT_IP, _RUN_USER, _RUN_IP  per-minute limits (defaults 10, 30, 20, 60;
//	                        a negative value turns that limit off)
//	LEETFORCE_METRICS_ADDR  Prometheus /metrics listen address (default "127.0.0.1:9102"; "off" disables it)
//	LEETFORCE_METRICS_QUEUE_EVERY  how often queue depth is sampled from Redis (default 60s; each sample is
//	                        5 Redis commands, so mind the hosted plan's monthly budget)
//	LEETFORCE_LIMIT_AUTH_IP, _LOGIN_ACCOUNT  per-10-minute sign-up/login limits (defaults 20, 10)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"leetforce/api/internal/catalog"
	"leetforce/api/internal/contest"
	"leetforce/api/internal/ingest"
	"leetforce/api/internal/metrics"
	"leetforce/api/internal/reaper"
	"leetforce/api/internal/rejudge"
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
	changes, err := rejudge.Sync(ctx, db, cat.Problems())
	if err != nil {
		return err
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
	// A problem whose tests changed: move its submissions to the new version
	// and queue them again (bundles are already published above).
	if err := rejudge.New(db, q, log, 0).RunChanged(ctx, changes); err != nil {
		log.Error("rejudge after problem sync; the reaper retries unqueued rows", "err", err)
	}

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

	limits, err := limitsFromEnv()
	if err != nil {
		return err
	}
	var proxies []string
	for _, p := range strings.Split(os.Getenv("LEETFORCE_TRUSTED_PROXIES"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			proxies = append(proxies, p)
		}
	}
	handler := server.New(server.Deps{Logger: log, Ready: ready, Problems: db, Samples: cat, Content: cat, Submissions: db,
		Queue: q, Versions: db, Runs: q, Accounts: db, Limiter: q, Limits: limits, Contests: contest.NewPG(db.Pool()), TrustedProxies: proxies})
	go sweepSessions(ctx, db, log)

	queueEvery, err := envDuration("LEETFORCE_METRICS_QUEUE_EVERY")
	if err != nil {
		return err
	}
	if queueEvery == 0 {
		queueEvery = time.Minute
	}
	go metrics.SampleQueue(ctx, q, queueEvery, log)
	metricsSrv := startMetrics(log)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		// Idle keep-alive connections are reaped. There is deliberately no ReadTimeout
		// or WriteTimeout: Go keeps the read deadline armed while a handler runs, so
		// either would cut the SSE streams, which carry their own limit
		// (EventConfig.MaxDuration).
		IdleTimeout: 2 * time.Minute,
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
	if metricsSrv != nil {
		_ = metricsSrv.Shutdown(shutdown)
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

// limitsFromEnv reads the optional rate-limit settings; unset keeps the default.
func limitsFromEnv() (server.Limits, error) {
	var l server.Limits
	for name, dst := range map[string]*int{
		"LEETFORCE_LIMIT_SUBMIT_USER":   &l.SubmitUser,
		"LEETFORCE_LIMIT_SUBMIT_IP":     &l.SubmitIP,
		"LEETFORCE_LIMIT_RUN_USER":      &l.RunUser,
		"LEETFORCE_LIMIT_RUN_IP":        &l.RunIP,
		"LEETFORCE_LIMIT_AUTH_IP":       &l.AuthIP,
		"LEETFORCE_LIMIT_LOGIN_ACCOUNT": &l.LoginAccount,
	} {
		v := os.Getenv(name)
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return l, fmt.Errorf("%s: %q is not an integer", name, v)
		}
		*dst = n
	}
	return l, nil
}

// sweepSessions deletes expired sessions once a day. Expired sessions are
// already refused at lookup; this only keeps the table small.
func sweepSessions(ctx context.Context, db *store.Store, log *slog.Logger) {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := db.DeleteExpiredSessions(ctx); err != nil {
				log.Warn("sweep sessions", "err", err)
			} else if n > 0 {
				log.Info("expired sessions removed", "count", n)
			}
		}
	}
}

// startMetrics serves /metrics on its own listener, bound to localhost by
// default, so the public API port never exposes it.
func startMetrics(log *slog.Logger) *http.Server {
	addr := os.Getenv("LEETFORCE_METRICS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:9102"
	}
	if addr == "off" {
		return nil
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics listener", "addr", addr, "err", err)
		}
	}()
	log.Info("metrics listening", "addr", addr)
	return srv
}
