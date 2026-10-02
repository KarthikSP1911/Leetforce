// Command api is the LeetForce HTTP API. Configuration comes from the environment:
//
//	DATABASE_URL            required, Neon Postgres URL
//	LEETFORCE_REDIS_URL     required, redis:// or rediss:// URL
//	LEETFORCE_API_ADDR      listen address (default ":8080")
//	LEETFORCE_PROBLEMS_DIR  problem directory (default "problems")
//	LEETFORCE_QUEUE_PREFIX  queue key prefix (default "leetforce")
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

	"leetforce/api/internal/server"
	"leetforce/api/internal/store"
	"leetforce/queue"
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

	srv := &http.Server{
		Addr:              addr,
		Handler:           server.New(server.Deps{Logger: log, Ready: map[string]server.Pinger{"database": db, "redis": q}}),
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
	log.Info("api stopped")
	return nil
}
