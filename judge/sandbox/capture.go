package sandbox

import (
	"bytes"
	"sync"
)

// cappedBuffer collects up to max bytes and silently discards the rest, so a
// program that floods its output can never grow host memory. It always reports
// the full write as accepted, so the writer is never blocked or sent a broken
// pipe; the first overflow calls onExceed (used to kill the run).
type cappedBuffer struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	max      int64
	exceeded bool
	onExceed func()
}

func newCappedBuffer(max int64, onExceed func()) *cappedBuffer {
	return &cappedBuffer{max: max, onExceed: onExceed}
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	remaining := c.max - int64(c.buf.Len())
	first := false
	if int64(len(p)) > remaining {
		if remaining > 0 {
			c.buf.Write(p[:remaining])
		}
		if !c.exceeded {
			c.exceeded = true
			first = true
		}
	} else {
		c.buf.Write(p)
	}
	c.mu.Unlock()

	if first && c.onExceed != nil {
		c.onExceed()
	}
	return len(p), nil
}

func (c *cappedBuffer) Bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.buf.Bytes()...)
}

func (c *cappedBuffer) Exceeded() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exceeded
}
