package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Security review (phase 16): a body that is not declared as JSON is refused on
// every state-changing route, so a cross-site HTML form (text/plain or
// form-urlencoded, which need no CORS preflight) cannot drive them.
func TestPostRoutesRequireJSONContentType(t *testing.T) {
	a := newFakeAccounts()
	d := authDeps(a)
	for _, path := range []string{"/auth/signup", "/auth/login", "/submissions", "/runs"} {
		for _, ct := range []string{"", "text/plain", "application/x-www-form-urlencoded", "multipart/form-data"} {
			w := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path,
				strings.NewReader(`{"login":"ada","password":"x","problem":"p","language":"python","source":"x"}`))
			if ct != "" {
				req.Header.Set("Content-Type", ct)
			}
			req.AddCookie(a.cookie())
			New(d).ServeHTTP(w, req)
			if w.Code != http.StatusUnsupportedMediaType {
				t.Errorf("POST %s with Content-Type %q = %d, want 415", path, ct, w.Code)
			}
		}
	}
	// A charset parameter is still JSON.
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/login", strings.NewReader(`{"login":"nobody","password":"x"}`))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	New(d).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("JSON with charset = %d, want 401 from the login handler", w.Code)
	}
}
