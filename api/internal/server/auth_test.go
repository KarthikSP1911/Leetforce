package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"leetforce/api/internal/store"
)

const fakeUserID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

// fakeAccounts is an in-memory AccountStore with one signed-in user.
type fakeAccounts struct {
	mu       sync.Mutex
	users    map[string]store.User // lower-cased email and username -> user
	sessions map[string]string     // token hash (as string) -> user id
	solved   map[string]bool
	token    string
}

func newFakeAccounts() *fakeAccounts {
	f := &fakeAccounts{users: map[string]store.User{}, sessions: map[string]string{}, token: "test-session-token"}
	u := store.User{ID: fakeUserID, Email: "ada@example.com", Username: "ada"}
	f.users["ada@example.com"], f.users["ada"] = u, u
	f.sessions[string(hashToken(f.token))] = u.ID
	return f
}

// cookie is the session cookie of the pre-made user.
func (f *fakeAccounts) cookie() *http.Cookie {
	return &http.Cookie{Name: sessionCookie, Value: f.token}
}

func (f *fakeAccounts) CreateUser(_ context.Context, u store.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, n := strings.ToLower(u.Email), strings.ToLower(u.Username)
	if _, ok := f.users[e]; ok {
		return store.ErrConflict
	}
	if _, ok := f.users[n]; ok {
		return store.ErrConflict
	}
	f.users[e], f.users[n] = u, u
	return nil
}

func (f *fakeAccounts) UserByLogin(_ context.Context, login string) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.users[strings.ToLower(login)]; ok {
		return u, nil
	}
	return store.User{}, store.ErrNotFound
}

func (f *fakeAccounts) CreateSession(_ context.Context, h []byte, userID string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[string(h)] = userID
	return nil
}

func (f *fakeAccounts) SessionUser(_ context.Context, h []byte) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.sessions[string(h)]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	for _, u := range f.users {
		if u.ID == id {
			return store.User{ID: u.ID, Email: u.Email, Username: u.Username}, nil
		}
	}
	return store.User{}, store.ErrNotFound
}

func (f *fakeAccounts) DeleteSession(_ context.Context, h []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, string(h))
	return nil
}

func (f *fakeAccounts) SolvedProblems(context.Context, string) (map[string]bool, error) {
	return f.solved, nil
}

// fakeLimiter counts hits per key and denies past limit, like the Redis one
// without the window.
type fakeLimiter struct {
	mu   sync.Mutex
	hits map[string]int
}

func (l *fakeLimiter) Allow(_ context.Context, key string, limit int, _ time.Duration) (bool, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[string]int{}
	}
	l.hits[key]++
	if l.hits[key] > limit {
		return false, 42 * time.Second, nil
	}
	return true, 0, nil
}

func authDeps(a *fakeAccounts) Deps {
	return Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Accounts: a}
}

func doJSON(d Deps, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.7:4000"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	New(d).ServeHTTP(w, req)
	return w
}

func sessionFrom(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	return nil
}

func TestSignupLoginLogout(t *testing.T) {
	a := newFakeAccounts()
	d := authDeps(a)

	w := doJSON(d, http.MethodPost, "/auth/signup", `{"email":"grace@example.com","username":"grace","password":"correct horse"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("signup = %d %s", w.Code, w.Body)
	}
	c := sessionFrom(w)
	if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Value == "" {
		t.Fatalf("session cookie = %+v, want HttpOnly SameSite=Lax with a value", c)
	}
	for _, leak := range []string{"password", "hash"} {
		if strings.Contains(strings.ToLower(w.Body.String()), leak) {
			t.Fatalf("signup response leaks %q: %s", leak, w.Body)
		}
	}
	// Only the hash of the token is stored.
	for stored := range a.sessions {
		if stored == c.Value {
			t.Fatal("the raw session token was stored")
		}
	}
	if w := doJSON(d, http.MethodGet, "/me", "", c); w.Code != 200 || !strings.Contains(w.Body.String(), `"username":"grace"`) {
		t.Fatalf("me = %d %s", w.Code, w.Body)
	}
	if w := doJSON(d, http.MethodGet, "/me", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous me = %d, want 401", w.Code)
	}

	if w := doJSON(d, http.MethodPost, "/auth/logout", "", c); w.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", w.Code)
	}
	if w := doJSON(d, http.MethodGet, "/me", "", c); w.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout = %d, want 401 (the session must be gone server-side)", w.Code)
	}

	// Log back in by email, then by username, case-insensitively. The fake
	// store keeps the hash signup computed, so the same password works.
	for _, login := range []string{"grace@example.com", "GRACE"} {
		w := doJSON(d, http.MethodPost, "/auth/login", fmt.Sprintf(`{"login":%q,"password":"correct horse"}`, login))
		if w.Code != http.StatusOK || sessionFrom(w) == nil {
			t.Fatalf("login %q = %d %s", login, w.Code, w.Body)
		}
	}
}

func TestSignupValidation(t *testing.T) {
	d := authDeps(newFakeAccounts())
	tests := []struct {
		name, body string
		want       int
	}{
		{"bad email", `{"email":"nope","username":"grace","password":"correct horse"}`, 422},
		{"email with display name", `{"email":"G <g@example.com>","username":"grace","password":"correct horse"}`, 422},
		{"short username", `{"email":"g@example.com","username":"gr","password":"correct horse"}`, 422},
		{"bad username chars", `{"email":"g@example.com","username":"gr ace","password":"correct horse"}`, 422},
		{"short password", `{"email":"g@example.com","username":"grace","password":"short"}`, 422},
		{"password over bcrypt limit", `{"email":"g@example.com","username":"grace","password":"` + strings.Repeat("a", 73) + `"}`, 422},
		{"taken email", `{"email":"ADA@example.com","username":"other","password":"correct horse"}`, 409},
		{"taken username", `{"email":"o@example.com","username":"Ada","password":"correct horse"}`, 409},
		{"not JSON", `nope`, 400},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(d, http.MethodPost, "/auth/signup", tc.body)
			if w.Code != tc.want || sessionFrom(w) != nil {
				t.Fatalf("status = %d (want %d), cookie %+v", w.Code, tc.want, sessionFrom(w))
			}
		})
	}
}

func TestLoginRejects(t *testing.T) {
	a := newFakeAccounts()
	hash, _ := bcrypt.GenerateFromPassword([]byte("right password"), 4)
	u := a.users["ada"]
	u.PasswordHash = string(hash)
	a.users["ada"], a.users["ada@example.com"] = u, u
	d := authDeps(a)

	if w := doJSON(d, http.MethodPost, "/auth/login", `{"login":"ada","password":"right password"}`); w.Code != 200 {
		t.Fatalf("good login = %d %s", w.Code, w.Body)
	}
	wrong := doJSON(d, http.MethodPost, "/auth/login", `{"login":"ada","password":"wrong password"}`)
	ghost := doJSON(d, http.MethodPost, "/auth/login", `{"login":"nobody","password":"wrong password"}`)
	if wrong.Code != 401 || ghost.Code != 401 || wrong.Body.String() != ghost.Body.String() {
		t.Fatalf("wrong password = %d %s, unknown account = %d %s; want identical 401s", wrong.Code, wrong.Body, ghost.Code, ghost.Body)
	}
}

func TestAuthRateLimits(t *testing.T) {
	d := authDeps(newFakeAccounts())
	d.Limiter = &fakeLimiter{}
	d.Limits = Limits{AuthIP: 3, LoginAccount: 2}
	attempt := func() *httptest.ResponseRecorder {
		return doJSON(d, http.MethodPost, "/auth/login", `{"login":"ada","password":"x"}`)
	}
	// The per-account limit (2) trips before the per-IP one (3).
	for i := 0; i < 2; i++ {
		if w := attempt(); w.Code != 401 {
			t.Fatalf("attempt %d = %d, want 401", i+1, w.Code)
		}
	}
	w := attempt()
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "42" {
		t.Fatalf("third attempt = %d (Retry-After %q), want 429 with 42", w.Code, w.Header().Get("Retry-After"))
	}
	var body struct{ Error string }
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Error == "" {
		t.Fatalf("429 body = %s", w.Body)
	}
}

func TestSubmitAndRunRequireLogin(t *testing.T) {
	subs, q := &fakeSubs{}, &fakeQueue{}
	d := Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Submissions: subs, Queue: q, Accounts: newFakeAccounts(),
		Runs: &fakeRuns{}, Versions: fakeVersions{}}
	body := `{"problem":"sum","language":"python","source":"print(3)"}`
	for _, path := range []string{"/submissions", "/runs"} {
		if w := doJSON(d, http.MethodPost, path, body); w.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous POST %s = %d, want 401", path, w.Code)
		}
		if w := doJSON(d, http.MethodPost, path, body, &http.Cookie{Name: sessionCookie, Value: "forged"}); w.Code != http.StatusUnauthorized {
			t.Fatalf("forged-cookie POST %s = %d, want 401", path, w.Code)
		}
	}
	if len(q.jobs) != 0 || len(subs.rows) != 0 {
		t.Fatal("an unauthenticated request must not store or queue anything")
	}
}

func TestSubmitRunLimits(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		limits Limits
	}{
		{"submit per user", "/submissions", Limits{SubmitUser: 2, SubmitIP: 100}},
		{"run per user", "/runs", Limits{RunUser: 2, RunIP: 100}},
	}
	body := `{"problem":"sample-sum","language":"python","source":"print(3)"}`
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newFakeAccounts()
			d := runDeps(&fakeQueue{}, &fakeRuns{}, fakeVersions{})
			d.Accounts, d.Submissions = a, &fakeSubs{}
			d.Limiter, d.Limits = &fakeLimiter{}, tc.limits
			for i := 1; i <= 2; i++ {
				if w := doJSON(d, http.MethodPost, tc.path, body, a.cookie()); w.Code != http.StatusAccepted {
					t.Fatalf("request %d = %d %s, want 202", i, w.Code, w.Body)
				}
			}
			// The same user from another IP is still limited: the limit is per user.
			w := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, tc.path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "198.51.100.9:1"
			req.AddCookie(a.cookie())
			New(d).ServeHTTP(w, req)
			if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
				t.Fatalf("third request = %d, want 429 with Retry-After", w.Code)
			}
		})
	}

	t.Run("per IP", func(t *testing.T) {
		a := newFakeAccounts()
		d := runDeps(&fakeQueue{}, &fakeRuns{}, fakeVersions{})
		d.Accounts = a
		d.Limiter, d.Limits = &fakeLimiter{}, Limits{RunUser: 100, RunIP: 1}
		if w := doJSON(d, http.MethodPost, "/runs", body, a.cookie()); w.Code != http.StatusAccepted {
			t.Fatalf("first = %d", w.Code)
		}
		if w := doJSON(d, http.MethodPost, "/runs", body, a.cookie()); w.Code != http.StatusTooManyRequests {
			t.Fatalf("second from the same IP = %d, want 429", w.Code)
		}
	})
}

func TestForwardedForIsIgnoredUnlessTrusted(t *testing.T) {
	a := newFakeAccounts()
	d := runDeps(&fakeQueue{}, &fakeRuns{}, fakeVersions{})
	d.Accounts = a
	d.Limiter, d.Limits = &fakeLimiter{}, Limits{RunUser: 100, RunIP: 1}
	body := `{"problem":"sample-sum","language":"python","source":"print(3)"}`
	send := func(xff string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/runs", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", xff)
		req.RemoteAddr = "203.0.113.7:4000"
		req.AddCookie(a.cookie())
		New(d).ServeHTTP(w, req)
		return w.Code
	}
	send("10.0.0.1")
	if code := send("10.0.0.2"); code != http.StatusTooManyRequests {
		t.Fatalf("a forged X-Forwarded-For escaped the per-IP limit: status %d", code)
	}
}

func TestProblemListMarksSolved(t *testing.T) {
	a := newFakeAccounts()
	a.solved = map[string]bool{"a-sum": true}
	d := Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Problems: fakeProblems{list: sampleProblems()}, Accounts: a}
	solvedOf := func(withCookie bool) map[string]bool {
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/problems", nil)
		if withCookie {
			req.AddCookie(a.cookie())
		}
		New(d).ServeHTTP(w, req)
		var page struct {
			Problems []struct {
				Slug   string
				Solved bool
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Problems) == 0 {
			t.Fatalf("list = %d %s", w.Code, w.Body)
		}
		out := map[string]bool{}
		for _, p := range page.Problems {
			out[p.Slug] = p.Solved
		}
		return out
	}
	if got := solvedOf(true); !got["a-sum"] {
		t.Fatalf("signed-in list = %v, want a-sum solved", got)
	}
	for slug, solved := range solvedOf(false) {
		if solved {
			t.Fatalf("anonymous list marks %q solved", slug)
		}
	}
}
