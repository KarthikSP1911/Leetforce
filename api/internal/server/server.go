// Package server builds the API's HTTP router.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"leetforce/api/internal/metrics"
)

// Pinger is a dependency that can report whether it is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps are the collaborators the router needs.
type Deps struct {
	Logger *slog.Logger
	// Ready lists the dependencies /readyz checks, by name (database, redis).
	Ready map[string]Pinger

	Problems    ProblemStore
	Samples     SampleSource
	Content     ContentSource // statement and starters; may be nil
	Submissions SubmissionStore
	Queue       Enqueuer
	Versions    VersionSource  // current test-set version, for Run jobs
	Runs        RunStore       // state of Run jobs (Redis, never Postgres)
	Accounts    AccountStore   // users and sessions
	Limiter     Limiter        // rate-limit counters; nil turns limits off
	Limits      Limits         // zero fields take defaults
	Contests    ContestService // contests and contest-only problems; nil turns them off

	// TrustedProxies are the addresses whose X-Forwarded-For header is believed
	// when finding the client IP (for example the Next.js proxy). Empty means
	// the TCP peer address is always used, so the header cannot be forged.
	TrustedProxies []string

	// Events tunes the SSE status stream; the zero value takes the defaults.
	Events EventConfig

	slots streamSlots
}

// New returns the router. /healthz says the process is up and does no I/O;
// /readyz says it can serve requests, which needs its dependencies.
func New(d Deps) *gin.Engine {
	d.slots = newStreamSlots(d.Events.withDefaults().MaxStreams)
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// Cannot fail for an empty list; a bad entry is reported when the API starts.
	_ = r.SetTrustedProxies(d.TrustedProxies)
	r.Use(gin.Recovery(), requestLog(d.Logger))

	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		checks := gin.H{}
		code := http.StatusOK
		for name, p := range d.Ready {
			if err := p.Ping(ctx); err != nil {
				// The error text can name hosts, so it goes to the log, not the response.
				d.Logger.Warn("readiness check failed", "dependency", name, "err", err)
				checks[name] = "down"
				code = http.StatusServiceUnavailable
				continue
			}
			checks[name] = "ok"
		}
		c.JSON(code, gin.H{"checks": checks})
	})

	r.POST("/auth/signup", d.signup)
	r.POST("/auth/login", d.login)
	r.POST("/auth/logout", d.logout)
	r.GET("/me", d.me)

	d.contestRoutes(r)

	r.GET("/problems", d.listProblems)
	r.GET("/problems/:slug", d.getProblem)
	r.GET("/problems/:slug/submissions", d.listSubmissions)
	r.POST("/submissions", d.createSubmission)
	r.GET("/submissions/:id", d.getSubmission)
	r.GET("/submissions/:id/events", d.streamEvents)
	r.POST("/runs", d.createRun)
	r.GET("/runs/:id", d.getRun)
	return r
}

func requestLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		elapsed := time.Since(start)
		// The route template, never the raw path, keeps the label set bounded.
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		metrics.HTTPRequests.WithLabelValues(c.Request.Method, route, strconv.Itoa(c.Writer.Status())).Inc()
		metrics.HTTPSeconds.WithLabelValues(c.Request.Method, route).Observe(elapsed.Seconds())
		log.Info("request", "method", c.Request.Method, "path", route,
			"status", c.Writer.Status(), "ms", elapsed.Milliseconds())
	}
}
