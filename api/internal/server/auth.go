package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"leetforce/api/internal/store"
)

const (
	sessionCookie = "lf_session"
	sessionTTL    = 30 * 24 * time.Hour
	bcryptCost    = 12
	minPassword   = 8
	// bcrypt ignores input past 72 bytes, so longer passwords are refused
	// instead of silently truncated.
	maxPassword  = 72
	maxAuthBody  = 4 << 10
	userContext  = "lf.user"
	noUserMarker = "lf.nouser"
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

// dummyHash is compared against when a login names no account, so a missing
// account costs the same time as a wrong password and cannot be told apart.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("leetforce-dummy-password"), bcryptCost)

// AccountStore holds accounts, sessions and the per-user views of submissions.
type AccountStore interface {
	CreateUser(ctx context.Context, u store.User) error
	UserByLogin(ctx context.Context, login string) (store.User, error)
	CreateSession(ctx context.Context, tokenHash []byte, userID string, expires time.Time) error
	SessionUser(ctx context.Context, tokenHash []byte) (store.User, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
	SolvedProblems(ctx context.Context, userID string) (map[string]bool, error)
}

type userView struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

func viewOf(u store.User) userView { return userView{ID: u.ID, Email: u.Email, Username: u.Username} }

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func secureRequest(c *gin.Context) bool {
	return c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
}

func setSessionCookie(c *gin.Context, token string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: secureRequest(c), SameSite: http.SameSiteLaxMode,
	})
}

// currentUser returns the account behind the request's session cookie. The
// lookup happens once per request; an unknown, expired or missing session is
// simply "not signed in".
func (d Deps) currentUser(c *gin.Context) (store.User, bool) {
	if v, ok := c.Get(userContext); ok {
		return v.(store.User), true
	}
	if _, done := c.Get(noUserMarker); done || d.Accounts == nil {
		return store.User{}, false
	}
	c.Set(noUserMarker, true)
	token, err := c.Cookie(sessionCookie)
	if err != nil || token == "" {
		return store.User{}, false
	}
	u, err := d.Accounts.SessionUser(c.Request.Context(), hashToken(token))
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			d.Logger.Error("session lookup", "err", err)
		}
		return store.User{}, false
	}
	c.Set(userContext, u)
	return u, true
}

// requireUser answers 401 and returns false when nobody is signed in.
func (d Deps) requireUser(c *gin.Context) (store.User, bool) {
	u, ok := d.currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "sign in required"})
	}
	return u, ok
}

// startSession creates a session for the user and sets the cookie.
func (d Deps) startSession(c *gin.Context, u store.User) bool {
	token, err := newToken()
	if err != nil {
		d.fail(c, "new session", err)
		return false
	}
	if err := d.Accounts.CreateSession(c.Request.Context(), hashToken(token), u.ID, time.Now().Add(sessionTTL)); err != nil {
		d.fail(c, "create session", err)
		return false
	}
	setSessionCookie(c, token, int(sessionTTL.Seconds()))
	return true
}

func bindAuth(c *gin.Context, req any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAuthBody)
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body must be JSON"})
		return false
	}
	return true
}

type signupRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// validateSignup returns a user-facing message, or "" if the input is fine.
func validateSignup(r signupRequest) string {
	if a, err := mail.ParseAddress(r.Email); err != nil || a.Address != r.Email || len(r.Email) > 254 {
		return "enter a valid email address"
	}
	if !usernameRe.MatchString(r.Username) {
		return "username must be 3-32 characters: letters, digits, _ or -"
	}
	if len(r.Password) < minPassword {
		return fmt.Sprintf("password must be at least %d characters", minPassword)
	}
	if len(r.Password) > maxPassword {
		return fmt.Sprintf("password must be at most %d bytes", maxPassword)
	}
	return ""
}

func (d Deps) signup(c *gin.Context) {
	if !d.limit(c, "auth-ip", c.ClientIP(), d.limits().AuthIP, d.limits().AuthWindow) {
		return
	}
	var req signupRequest
	if !bindAuth(c, &req) {
		return
	}
	req.Email, req.Username = strings.TrimSpace(req.Email), strings.TrimSpace(req.Username)
	if msg := validateSignup(req); msg != "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": msg})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		d.fail(c, "hash password", err)
		return
	}
	u := store.User{ID: uuid.NewString(), Email: req.Email, Username: req.Username, PasswordHash: string(hash)}
	if err := d.Accounts.CreateUser(c.Request.Context(), u); err != nil {
		if errors.Is(err, store.ErrConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "that email or username is already taken"})
			return
		}
		d.fail(c, "create user", err)
		return
	}
	if !d.startSession(c, u) {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": viewOf(u)})
}

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (d Deps) login(c *gin.Context) {
	lim := d.limits()
	if !d.limit(c, "auth-ip", c.ClientIP(), lim.AuthIP, lim.AuthWindow) {
		return
	}
	var req loginRequest
	if !bindAuth(c, &req) {
		return
	}
	req.Login = strings.TrimSpace(req.Login)
	if req.Login == "" || req.Password == "" || len(req.Password) > maxPassword {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "wrong email, username or password"})
		return
	}
	// Per-account limit: stops a distributed guessing run against one account.
	if !d.limit(c, "login-account", strings.ToLower(req.Login), lim.LoginAccount, lim.AuthWindow) {
		return
	}
	u, err := d.Accounts.UserByLogin(c.Request.Context(), req.Login)
	hash := dummyHash
	if err == nil {
		hash = []byte(u.PasswordHash)
	} else if !errors.Is(err, store.ErrNotFound) {
		d.fail(c, "user by login", err)
		return
	}
	match := bcrypt.CompareHashAndPassword(hash, []byte(req.Password)) == nil
	if err != nil || !match {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "wrong email, username or password"})
		return
	}
	if !d.startSession(c, u) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": viewOf(u)})
}

func (d Deps) logout(c *gin.Context) {
	if token, err := c.Cookie(sessionCookie); err == nil && token != "" {
		if err := d.Accounts.DeleteSession(c.Request.Context(), hashToken(token)); err != nil {
			d.Logger.Error("delete session", "err", err)
		}
	}
	setSessionCookie(c, "", -1)
	c.Status(http.StatusNoContent)
}

func (d Deps) me(c *gin.Context) {
	u, ok := d.requireUser(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": viewOf(u)})
}
