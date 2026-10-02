package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// VerdictRecord is a verdict to store. It carries no test data, compiler
// output or stderr: Submit never returns them, so they are not kept.
type VerdictRecord struct {
	SubmissionID string
	Verdict      string
	RuntimeMS    int64
	MemoryKB     int64
	Passed       int
	Total        int
	// TestSetVersion is the version the runner judged against. Empty means
	// unknown (an internal error): the submission's own version is used.
	TestSetVersion string
	RunnerID       string
}

// RecordVerdict stores a verdict and marks the submission judged, once.
//
// It is idempotent per submission: the verdicts primary key plus ON CONFLICT
// DO NOTHING make a second write a no-op, even with a different verdict, and
// the status update runs only when the insert happened, all in one statement
// so nothing can be half applied. It returns true if this call stored the
// verdict, and false for a duplicate or an unknown submission.
func (s *Store) RecordVerdict(ctx context.Context, v VerdictRecord) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		WITH ins AS (
			INSERT INTO verdicts (submission_id, verdict, runtime_ms, memory_kb, passed, total, test_set_version, runner_id)
			SELECT s.id, $2, $3, $4, $5, $6, COALESCE(NULLIF($7, ''), s.test_set_version), $8
			FROM submissions s WHERE s.id = $1::uuid
			ON CONFLICT (submission_id) DO NOTHING
			RETURNING submission_id
		)
		UPDATE submissions SET status = 'judged', updated_at = now()
		WHERE id IN (SELECT submission_id FROM ins)`,
		v.SubmissionID, v.Verdict, v.RuntimeMS, v.MemoryKB, v.Passed, v.Total, v.TestSetVersion, v.RunnerID)
	if err != nil {
		return false, fmt.Errorf("record verdict %s: %w", v.SubmissionID, err)
	}
	return tag.RowsAffected() == 1, nil
}

// IsPermanent reports whether err means the data itself is invalid (a bad
// UUID, a value the schema rejects), so retrying the same write can never
// succeed. Connection and timeout errors are not permanent.
func IsPermanent(err error) bool {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return false
	}
	// class 22 = data exception (invalid uuid text), 23 = integrity constraint violation
	return len(pg.Code) == 5 && (pg.Code[:2] == "22" || pg.Code[:2] == "23")
}
