package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"leetforce/api/internal/leaderboard"
)

// RankingService serves the scoreboards (leaderboard.Service).
type RankingService interface {
	Standings(ctx context.Context, slug string) (leaderboard.ContestStandings, error)
	Global(ctx context.Context, page, perPage int) (leaderboard.GlobalPage, error)
}

// getStandings serves GET /contests/:slug/standings.
func (d Deps) getStandings(c *gin.Context) {
	if d.Ranking == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "standings are not available"})
		return
	}
	st, err := d.Ranking.Standings(c.Request.Context(), c.Param("slug"))
	switch {
	case errors.Is(err, leaderboard.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "contest not found"})
	case errors.Is(err, leaderboard.ErrNoScorer):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "standings are not available"})
	case err != nil:
		d.Logger.Error("standings", "contest", c.Param("slug"), "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	default:
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, st)
	}
}

// getLeaderboard serves GET /leaderboard?page=&per_page=.
func (d Deps) getLeaderboard(c *gin.Context) {
	if d.Ranking == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "leaderboard is not available"})
		return
	}
	page, _ := strconv.Atoi(c.Query("page"))
	per, _ := strconv.Atoi(c.Query("per_page"))
	out, err := d.Ranking.Global(c.Request.Context(), page, per)
	if err != nil {
		d.Logger.Error("leaderboard", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, out)
}
