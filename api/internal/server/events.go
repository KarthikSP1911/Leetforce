package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"leetforce/api/internal/metrics"
	"leetforce/api/internal/store"
)

// EventConfig tunes GET /submissions/:id/events. Zero values take the defaults
// noted per field; tests set small durations.
type EventConfig struct {
	PollEvery   time.Duration // how often a stream reads the submission (default 500ms)
	Heartbeat   time.Duration // idle keep-alive comment interval (default 15s)
	MaxDuration time.Duration // longest a stream stays open (default 10m)
	MaxStreams  int           // open streams per API instance (default 200)
	// MaxStreamsPerIP bounds open streams per client IP (default 10), so one
	// client cannot take every slot of the instance.
	MaxStreamsPerIP int
	MaxErrors       int // consecutive read failures before giving up (default 10)
}

func (c EventConfig) withDefaults() EventConfig {
	if c.PollEvery <= 0 {
		c.PollEvery = 500 * time.Millisecond
	}
	if c.Heartbeat <= 0 {
		c.Heartbeat = 15 * time.Second
	}
	if c.MaxDuration <= 0 {
		c.MaxDuration = 10 * time.Minute
	}
	if c.MaxStreams <= 0 {
		c.MaxStreams = 200
	}
	if c.MaxStreamsPerIP <= 0 {
		c.MaxStreamsPerIP = 10
	}
	if c.MaxErrors <= 0 {
		c.MaxErrors = 10
	}
	return c
}

// streamSlots bounds concurrent streams, in total and per client IP. Each
// stream polls the database, so an unbounded number would let a few clients
// keep the pool busy, and without the per-IP bound one client could hold every
// slot and lock everyone else out.
type streamSlots struct {
	total chan struct{}
	perIP int

	mu  sync.Mutex
	ips map[string]int
}

func newStreamSlots(n, perIP int) *streamSlots {
	return &streamSlots{total: make(chan struct{}, n), perIP: perIP, ips: map[string]int{}}
}

// acquire takes a slot for ip; it reports false when the instance or that IP is full.
func (s *streamSlots) acquire(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ips[ip] >= s.perIP {
		return false
	}
	select {
	case s.total <- struct{}{}:
		s.ips[ip]++
		return true
	default:
		return false
	}
}

func (s *streamSlots) release(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	<-s.total
	if s.ips[ip]--; s.ips[ip] <= 0 {
		delete(s.ips, ip)
	}
}

// statusEvent is the payload of "status" and "verdict" events. It carries the
// state and, once judged, the same verdict view GET /submissions/:id returns:
// never the source, test data, test-set version or stderr.
type statusEvent struct {
	Status  string             `json:"status"`
	Verdict *store.VerdictView `json:"verdict,omitempty"`
}

// streamEvents is a Server-Sent Events stream of one submission's state:
// "status" events (queued, then judging) whenever the state changes, then a
// "verdict" event and the end of the stream. The first event is the current
// state, so a client that connects late, or reconnects, sees where things
// stand; nothing depends on Last-Event-ID. State is read from Postgres, which
// is the source of truth for every API instance.
func (d Deps) streamEvents(c *gin.Context) {
	cfg := d.Events.withDefaults()
	ctx := c.Request.Context()

	sub, err := d.Submissions.GetSubmission(ctx, c.Param("id"))
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		return
	}
	if err != nil {
		d.fail(c, "stream submission", err)
		return
	}
	ip := c.ClientIP()
	if !d.slots.acquire(ip) {
		metrics.StreamsRefused.Inc()
		c.Header("Retry-After", "5")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "too many open streams, try again"})
		return
	}
	metrics.StreamsOpen.Inc()
	defer func() {
		metrics.StreamsOpen.Dec()
		d.slots.release(ip)
	}()

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming is not supported"})
		return
	}
	h := c.Writer.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // tell nginx-style proxies not to buffer
	c.Status(http.StatusOK)

	send := func(event string, v any) bool {
		b, err := json.Marshal(v)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if _, err := fmt.Fprint(c.Writer, "retry: 3000\n\n"); err != nil {
		return
	}

	last := ""
	emit := func(s store.Submission) (done, alive bool) {
		ev := statusEvent{Status: s.Status}
		if s.Status == store.StatusJudged && s.Verdict != nil {
			ev.Verdict = s.Verdict
			return true, send("verdict", ev)
		}
		if s.Status == last {
			return false, true
		}
		last = s.Status
		return false, send("status", ev)
	}
	if done, alive := emit(sub); done || !alive {
		return
	}

	poll := time.NewTicker(cfg.PollEvery)
	defer poll.Stop()
	beat := time.NewTicker(cfg.Heartbeat)
	defer beat.Stop()
	deadline := time.NewTimer(cfg.MaxDuration)
	defer deadline.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			send("timeout", gin.H{"error": "stream time limit reached, reconnect to continue"})
			return
		case <-beat.C:
			if _, err := fmt.Fprint(c.Writer, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-poll.C:
			s, err := d.Submissions.GetSubmission(ctx, sub.ID)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				failures++
				d.Logger.Warn("stream read failed", "id", sub.ID, "failures", failures, "err", err)
				if failures >= cfg.MaxErrors {
					send("error", gin.H{"error": "status is unavailable, try again"})
					return
				}
				continue
			}
			failures = 0
			if done, alive := emit(s); done || !alive {
				return
			}
		}
	}
}
