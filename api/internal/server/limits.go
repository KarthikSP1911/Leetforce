package server

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Limiter counts hits against a key in a fixed window. queue.Queue implements
// it on Redis so every API instance shares the counters.
type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (ok bool, retryAfter time.Duration, err error)
}

// Limits are the rate limits. A zero field takes its default; a negative one
// turns that limit off.
type Limits struct {
	SubmitUser, SubmitIP int
	SubmitWindow         time.Duration
	RunUser, RunIP       int
	RunWindow            time.Duration
	AuthIP, LoginAccount int
	AuthWindow           time.Duration
}

func (l Limits) withDefaults() Limits {
	def := func(v *int, d int) {
		if *v == 0 {
			*v = d
		}
	}
	defw := func(v *time.Duration, d time.Duration) {
		if *v == 0 {
			*v = d
		}
	}
	def(&l.SubmitUser, 10)
	def(&l.SubmitIP, 30)
	defw(&l.SubmitWindow, time.Minute)
	def(&l.RunUser, 20)
	def(&l.RunIP, 60)
	defw(&l.RunWindow, time.Minute)
	def(&l.AuthIP, 20)
	def(&l.LoginAccount, 10)
	defw(&l.AuthWindow, 10*time.Minute)
	return l
}

func (d Deps) limits() Limits { return d.Limits.withDefaults() }

// limit counts one hit for scope/key and answers 429 with Retry-After when the
// caller is over limit. It returns true if the request may go on. With no
// Limiter configured, or a non-positive limit, nothing is enforced.
func (d Deps) limit(c *gin.Context, scope, key string, limit int, window time.Duration) bool {
	if d.Limiter == nil || limit <= 0 {
		return true
	}
	ok, retry, err := d.Limiter.Allow(c.Request.Context(), scope+":"+key, limit, window)
	if err != nil {
		// Without the counter we cannot tell abuse from use. The queue is on the
		// same Redis, so a submission would fail anyway: fail closed.
		d.Logger.Error("rate limit", "scope", scope, "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "try again in a moment"})
		return false
	}
	if !ok {
		secs := max(1, int(math.Ceil(retry.Seconds())))
		c.Header("Retry-After", strconv.Itoa(secs))
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests, retry in " + strconv.Itoa(secs) + "s"})
		return false
	}
	return true
}

// limitUserAndIP applies a per-user and a per-IP limit for one action.
func (d Deps) limitUserAndIP(c *gin.Context, action, userID string, userLimit, ipLimit int, window time.Duration) bool {
	return d.limit(c, action+"-user", userID, userLimit, window) &&
		d.limit(c, action+"-ip", c.ClientIP(), ipLimit, window)
}
