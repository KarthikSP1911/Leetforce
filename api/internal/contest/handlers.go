package contest

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler serves the contest endpoints.
type Handler struct {
	Store  Store
	Logger *slog.Logger
	// UserID returns the signed-in user's id, or false for anonymous callers.
	UserID func(c *gin.Context) (string, bool)
	// RequireUser is like UserID but answers 401 itself when nobody is signed in.
	RequireUser func(c *gin.Context) (string, bool)
	Now         func() time.Time // nil means time.Now
}

// Routes registers the contest endpoints.
func (h Handler) Routes(r gin.IRoutes) {
	r.GET("/contests", h.list)
	r.GET("/contests/:slug", h.get)
	r.POST("/contests/:slug/register", h.register)
	r.GET("/contests/:slug/problems", h.problems)
}

func (h Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h Handler) uid(c *gin.Context) string {
	id, _ := h.UserID(c)
	return id
}

func (h Handler) fail(c *gin.Context, what string, err error) {
	h.Logger.Error(what, "err", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}

func (h Handler) list(c *gin.Context) {
	cs, err := h.Store.ListContests(c.Request.Context(), h.uid(c))
	if err != nil {
		h.fail(c, "list contests", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"contests": cs})
}

type contestDetail struct {
	Contest
	ServerTime time.Time `json:"server_time"`
}

func (h Handler) get(c *gin.Context) {
	ct, err := h.Store.GetContest(c.Request.Context(), c.Param("slug"), h.uid(c))
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "contest not found"})
		return
	}
	if err != nil {
		h.fail(c, "get contest", err)
		return
	}
	c.JSON(http.StatusOK, contestDetail{Contest: ct, ServerTime: h.now()})
}

func (h Handler) register(c *gin.Context) {
	userID, ok := h.RequireUser(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	ct, err := h.Store.GetContest(ctx, c.Param("slug"), userID)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "contest not found"})
		return
	}
	if err != nil {
		h.fail(c, "get contest", err)
		return
	}
	if ct.Status == StatusEnded {
		c.JSON(http.StatusConflict, gin.H{"error": "contest has ended"})
		return
	}
	if err := h.Store.Register(ctx, ct.ID, userID); err != nil {
		h.fail(c, "register", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"registered": true})
}

// problems lists a contest's problems. Before the start, and during the contest
// for anyone not registered, the answer is 404 so the endpoint does not reveal
// what the problems are; after the end it is public.
func (h Handler) problems(c *gin.Context) {
	ctx := c.Request.Context()
	userID, _ := h.UserID(c)
	ct, err := h.Store.GetContest(ctx, c.Param("slug"), userID)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "contest not found"})
		return
	}
	if err != nil {
		h.fail(c, "get contest", err)
		return
	}
	if ct.Status == StatusUpcoming || (ct.Status == StatusRunning && !ct.Registered) {
		c.JSON(http.StatusNotFound, gin.H{"error": "contest not found"})
		return
	}
	ps, err := h.Store.Problems(ctx, ct.ID)
	if err != nil {
		h.fail(c, "contest problems", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"problems": ps})
}
