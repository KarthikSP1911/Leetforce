package contest

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakeStore struct {
	c          Contest
	registered []string
}

func (f *fakeStore) ListContests(context.Context, string) ([]Contest, error) {
	return []Contest{f.c}, nil
}

func (f *fakeStore) GetContest(_ context.Context, slug, _ string) (Contest, error) {
	if slug != f.c.Slug {
		return Contest{}, ErrNotFound
	}
	return f.c, nil
}
func (f *fakeStore) Events(context.Context, string) ([]Event, error) { return nil, nil }
func (f *fakeStore) IsRegistered(context.Context, string, string) (bool, error) {
	return f.c.Registered, nil
}
func (f *fakeStore) Problems(context.Context, string) ([]Problem, error) {
	return []Problem{{Label: "A", Position: 1, Points: 100, Slug: "two-sum", Title: "Two Sum", Difficulty: "easy"}}, nil
}
func (f *fakeStore) Register(_ context.Context, _, userID string) error {
	f.registered = append(f.registered, userID)
	return nil
}

func serve(f *fakeStore, signedIn bool, method, path string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	user := func(c *gin.Context) (string, bool) { return "u1", signedIn }
	Handler{
		Store: f, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), UserID: user,
		RequireUser: func(c *gin.Context) (string, bool) {
			if !signedIn {
				c.JSON(http.StatusUnauthorized, gin.H{})
			}
			return "u1", signedIn
		},
	}.Routes(r)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func contestWith(status string, registered bool) *fakeStore {
	return &fakeStore{c: Contest{ID: "id", Slug: "mock", Title: "Mock", Status: status, Registered: registered,
		StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour)}}
}

func TestProblemsVisibility(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		registered bool
		want       int
	}{
		{"before start hidden even if registered", StatusUpcoming, true, http.StatusNotFound},
		{"running, unregistered hidden", StatusRunning, false, http.StatusNotFound},
		{"running, registered visible", StatusRunning, true, http.StatusOK},
		{"ended public", StatusEnded, false, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serve(contestWith(tt.status, tt.registered), true, "GET", "/contests/mock/problems").Code; got != tt.want {
				t.Fatalf("status %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		signedIn bool
		path     string
		want     int
	}{
		{"upcoming ok", StatusUpcoming, true, "/contests/mock/register", http.StatusOK},
		{"running ok", StatusRunning, true, "/contests/mock/register", http.StatusOK},
		{"ended conflict", StatusEnded, true, "/contests/mock/register", http.StatusConflict},
		{"anonymous", StatusUpcoming, false, "/contests/mock/register", http.StatusUnauthorized},
		{"unknown", StatusUpcoming, true, "/contests/nope/register", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := contestWith(tt.status, false)
			if got := serve(f, tt.signedIn, "POST", tt.path).Code; got != tt.want {
				t.Fatalf("status %d, want %d", got, tt.want)
			}
			if tt.want != http.StatusOK && len(f.registered) != 0 {
				t.Fatal("registered despite refusal")
			}
		})
	}
}
