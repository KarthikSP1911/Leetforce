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

// RecordVerdict stores a verdict and marks the submission judged.
//
// It is idempotent per submission: a second write of the same verdict, or any
// conflicting one for the same test-set version, changes nothing. The one
// exception is a rejudge: a stored verdict is replaced when the incoming one
// was judged against the submission's current test_set_version and the stored
// one was judged against a different version (BeginRejudge moved the
// submission onto the new version). After the replacement the stored version
// equals the current one, so a redelivery of the same rejudge verdict is a
// no-op, and so is a late delivery of the old verdict.
//
// Two more rules keep a rejudge from making things worse. A verdict for a
// version other than the submission's own is dropped (its job was already
// superseded). An internal-error verdict (IE: a dead letter, or a host that
// gave up) never replaces a real one, so a failed rejudge keeps the previous
// verdict; it only moves a submission that BeginRejudge put back to queued
// onto judged again, so it does not wait for a job that will never finish.
//
// Everything is one statement, so nothing can be half applied. It returns
// true if this call stored or replaced the verdict, and false for a duplicate,
// a superseded verdict or an unknown submission.
func (s *Store) RecordVerdict(ctx context.Context, v VerdictRecord) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		WITH sub AS (
			SELECT id, test_set_version FROM submissions WHERE id = $1::uuid
		), up AS (
			INSERT INTO verdicts (submission_id, verdict, runtime_ms, memory_kb, passed, total, test_set_version, runner_id)
			SELECT sub.id, $2, $3, $4, $5, $6, COALESCE(NULLIF($7, ''), sub.test_set_version), $8
			FROM sub
			WHERE NULLIF($7, '') IS NULL OR $7 = sub.test_set_version
			ON CONFLICT (submission_id) DO UPDATE
			SET verdict = EXCLUDED.verdict, runtime_ms = EXCLUDED.runtime_ms, memory_kb = EXCLUDED.memory_kb,
			    passed = EXCLUDED.passed, total = EXCLUDED.total, test_set_version = EXCLUDED.test_set_version,
			    runner_id = EXCLUDED.runner_id, created_at = now()
			WHERE EXCLUDED.verdict <> 'IE'
			  AND verdicts.test_set_version <> EXCLUDED.test_set_version
			  AND EXCLUDED.test_set_version = (SELECT test_set_version FROM sub)
			RETURNING submission_id
		)
		UPDATE submissions SET status = 'judged', updated_at = now()
		WHERE id IN (SELECT submission_id FROM up)`,
		v.SubmissionID, v.Verdict, v.RuntimeMS, v.MemoryKB, v.Passed, v.Total, v.TestSetVersion, v.RunnerID)
	if err != nil {
		return false, fmt.Errorf("record verdict %s: %w", v.SubmissionID, err)
	}
	if tag.RowsAffected() == 1 {
		return true, nil
	}
	if v.Verdict == "IE" {
		// A failed rejudge keeps the old verdict but must not leave the
		// submission queued forever.
		if _, err := s.pool.Exec(ctx, `
			UPDATE submissions SET status = 'judged', updated_at = now()
			WHERE id = $1::uuid AND status <> 'judged'
			  AND (NULLIF($2, '') IS NULL OR $2 = test_set_version)
			  AND EXISTS (SELECT 1 FROM verdicts WHERE submission_id = $1::uuid)`,
			v.SubmissionID, v.TestSetVersion); err != nil {
			return false, fmt.Errorf("record verdict %s: %w", v.SubmissionID, err)
		}
	}
	return false, nil
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
