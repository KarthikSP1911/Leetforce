// Command runner pulls jobs from the Redis queue, judges them in the sandbox and
// reports verdicts. Configuration comes from the environment:
//
//	LEETFORCE_REDIS_URL         required, redis:// or rediss:// URL
//	LEETFORCE_PROBLEMS_DIR      problem directory (default "problems"); used only without object storage
//	LEETFORCE_S3_ENDPOINT       object storage host:port; when set, tests are fetched from the bucket
//	LEETFORCE_S3_ACCESS_KEY, LEETFORCE_S3_SECRET_KEY, LEETFORCE_S3_BUCKET, LEETFORCE_S3_USE_TLS
//	LEETFORCE_PROBLEM_CACHE     where fetched problems are unpacked (default <tmp>/leetforce-problems)
//	LEETFORCE_QUEUE_PREFIX      key prefix (default "leetforce"; tests use a throwaway one)
//	LEETFORCE_RUNNER_ID         consumer name (default "<hostname>-<pid>")
//	LEETFORCE_JOB_MIN_IDLE      idle time before a job is reclaimed (default 30s)
//	LEETFORCE_JOB_MAX_ATTEMPTS  deliveries before IE / dead letter (default 3)
//
//	LEETFORCE_METRICS_ADDR      Prometheus /metrics listen address (default "127.0.0.1:9101"; "off" disables it)
//	LEETFORCE_CGROUP_ROOT       cgroup v2 directory for per-run cgroups (root default /sys/fs/cgroup/leetforce)
//
// The runner runs either as root (dev host, sudo) or as an unprivileged user
// started by the hardened systemd unit in scripts/runner (ADR 0014): then it
// needs a delegated cgroup (Delegate=yes) and the AppArmor userns profile for
// nsjail on Ubuntu 24.04. It never needs root capabilities of its own.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"leetforce/judge/engine"
	"leetforce/judge/sandbox"
	"leetforce/queue"
	"leetforce/runner/internal/agent"
	"leetforce/runner/internal/metrics"
	"leetforce/runner/internal/problems"
	"leetforce/storage"
	"leetforce/telemetry"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "runner:", err)
		os.Exit(1)
	}
}

func run() error {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	url := os.Getenv("LEETFORCE_REDIS_URL")
	if url == "" {
		return fmt.Errorf("LEETFORCE_REDIS_URL is not set")
	}
	cgroupRoot, err := cgroupRoot(log)
	if err != nil {
		return err
	}
	minIdle, err := envDuration("LEETFORCE_JOB_MIN_IDLE", 30*time.Second)
	if err != nil {
		return err
	}
	attempts, err := envInt("LEETFORCE_JOB_MAX_ATTEMPTS", 3)
	if err != nil {
		return err
	}
	id := os.Getenv("LEETFORCE_RUNNER_ID")
	if id == "" {
		host, _ := os.Hostname()
		id = fmt.Sprintf("%s-%d", host, os.Getpid())
	}
	problemsDir := os.Getenv("LEETFORCE_PROBLEMS_DIR")
	if problemsDir == "" {
		problemsDir = "problems"
	}

	q, err := queue.Open(url, queue.Config{Prefix: os.Getenv("LEETFORCE_QUEUE_PREFIX"), MinIdle: minIdle, MaxDeliveries: int64(attempts)})
	if err != nil {
		return err
	}
	defer func() { _ = q.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	flushTraces, err := telemetry.Init(ctx, "runner")
	if err != nil {
		return err
	}
	defer func() {
		flush, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = flushTraces(flush)
	}()
	if err := q.Ping(ctx); err != nil {
		return fmt.Errorf("connect to redis: %w", err)
	}
	if err := q.Setup(ctx); err != nil {
		return err
	}

	var src problems.Source = problems.Dir{Root: problemsDir}
	s3cfg, useS3, err := storage.ConfigFromEnv()
	if err != nil {
		return err
	}
	if useS3 {
		st, err := storage.Open(s3cfg)
		if err != nil {
			return err
		}
		if err := st.Ping(ctx); err != nil {
			return fmt.Errorf("connect to object storage: %w", err)
		}
		cache := os.Getenv("LEETFORCE_PROBLEM_CACHE")
		if cache == "" {
			cache = filepath.Join(os.TempDir(), "leetforce-problems")
		}
		src = problems.S3{Store: st, Cache: cache}
		log.Info("problems from object storage", "bucket", s3cfg.Bucket, "cache", cache)
	} else {
		log.Info("problems from directory", "dir", problemsDir)
	}

	a := agent.New(q, &engine.Engine{CgroupRoot: cgroupRoot}, agent.Config{
		ID:             id,
		Problems:       src,
		HeartbeatEvery: minIdle / 3,
		MaxAttempts:    int64(attempts),
		Logger:         log,
	})
	metricsAddr := os.Getenv("LEETFORCE_METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = "127.0.0.1:9101"
	}
	metrics.Started.SetToCurrentTime()
	metrics.LastPoll.SetToCurrentTime()
	msrv := metrics.Serve(metricsAddr, log)
	err = a.Run(ctx)
	if msrv != nil {
		_ = msrv.Close()
	}
	return err
}

// cgroupRoot picks the cgroup directory for per-run cgroups. An explicit
// LEETFORCE_CGROUP_ROOT wins; an unprivileged runner prepares its delegated
// cgroup; root uses the sandbox default (empty string).
func cgroupRoot(log *slog.Logger) (string, error) {
	if v := os.Getenv("LEETFORCE_CGROUP_ROOT"); v != "" {
		return v, nil
	}
	if sandbox.Rootless() {
		root, err := sandbox.PrepareDelegatedRoot()
		if err != nil {
			return "", fmt.Errorf("unprivileged runner needs a delegated cgroup (systemd Delegate=yes): %w", err)
		}
		log.Info("running unprivileged", "uid", os.Geteuid(), "cgroup_root", root)
		return root, nil
	}
	return "", nil
}

func envDuration(name string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s: %q is not a positive duration", name, v)
	}
	return d, nil
}

func envInt(name string, def int) (int, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s: %q is not a positive integer", name, v)
	}
	return n, nil
}
