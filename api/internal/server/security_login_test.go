package server

import (
	"net/http"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// Naming one account by email and by username must not give two guess budgets.
func TestLoginLimitCountsAccountNotSpelling(t *testing.T) {
	a := newFakeAccounts()
	hash, _ := bcrypt.GenerateFromPassword([]byte("right password"), 4)
	u := a.users["ada"]
	u.PasswordHash = string(hash)
	a.users["ada"], a.users["ada@example.com"] = u, u
	d := authDeps(a)
	d.Limiter = &fakeLimiter{}
	d.Limits = Limits{AuthIP: 100, LoginAccount: 2}

	codes := []int{}
	for _, login := range []string{"ada", "ada@example.com", "ada", "ada@example.com"} {
		w := doJSON(d, http.MethodPost, "/auth/login", `{"login":"`+login+`","password":"wrong password"}`)
		codes = append(codes, w.Code)
	}
	want := []int{401, 401, 429, 429}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("attempt statuses = %v, want %v", codes, want)
		}
	}
}
