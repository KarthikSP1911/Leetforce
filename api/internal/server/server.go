// Package server builds the API's HTTP router.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
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
}

// New returns the router. /healthz says the process is up and does no I/O;
// /readyz says it can serve requests, which needs its dependencies.
func New(d Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
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
	return r
}

func requestLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("request", "method", c.Request.Method, "path", c.FullPath(),
			"status", c.Writer.Status(), "ms", time.Since(start).Milliseconds())
	}
}
