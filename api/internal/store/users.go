package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrConflict means a unique value (email or username) is already taken.
var ErrConflict = errors.New("store: already exists")

// User is an account. The password hash never leaves the store layer's callers
// except for the login check.
type User struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
}

// CreateUser inserts an account, or returns ErrConflict if the email or
// username (compared case-insensitively) is taken.
func (s *Store) CreateUser(ctx context.Context, u User) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users (id, email, username, password_hash) VALUES ($1, $2, $3, $4)`,
		u.ID, u.Email, u.Username, u.PasswordHash)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// UserByLogin finds an account by email or username, or returns ErrNotFound.
func (s *Store) UserByLogin(ctx context.Context, login string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, email, username, password_hash FROM users
		WHERE lower(email) = lower($1) OR lower(username) = lower($1)`, login).
		Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("user by login: %w", err)
	}
	return u, nil
}

// CreateSession stores a session by the hash of its token.
func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, userID string, expires time.Time) error {
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		tokenHash, userID, expires); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// SessionUser returns the owner of an unexpired session, or ErrNotFound.
func (s *Store) SessionUser(ctx context.Context, tokenHash []byte) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id::text, u.email, u.username FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, tokenHash).
		Scan(&u.ID, &u.Email, &u.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("session user: %w", err)
	}
	return u, nil
}

// DeleteSession ends a session; an unknown token is not an error.
func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes sessions past their expiry and returns how many.
func (s *Store) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// SolvedProblems returns the slugs of problems the user has an accepted
// submission for.
func (s *Store) SolvedProblems(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT s.problem_slug FROM submissions s JOIN verdicts v ON v.submission_id = s.id
		WHERE s.user_id = $1::uuid AND v.verdict = 'AC'`, userID)
	if err != nil {
		return nil, fmt.Errorf("solved problems: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, fmt.Errorf("solved problems: %w", err)
		}
		out[slug] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("solved problems: %w", err)
	}
	return out, nil
}
