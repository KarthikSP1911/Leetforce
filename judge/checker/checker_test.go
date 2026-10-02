package checker

import (
	"testing"

	"leetforce/judge/problem"
	"leetforce/judge/verdict"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		expected string
		actual   string
		want     verdict.Verdict
	}{
		{"tokens: identical", problem.CheckerTokens, "6\n", "6\n", verdict.AC},
		{"tokens: missing final newline", problem.CheckerTokens, "6\n", "6", verdict.AC},
		{"tokens: extra blank lines and spaces", problem.CheckerTokens, "1 2 3\n", "  1\n\n2   3 \n\n", verdict.AC},
		{"tokens: crlf", problem.CheckerTokens, "1 2\n3\n", "1 2\r\n3\r\n", verdict.AC},
		{"tokens: both empty", problem.CheckerTokens, "", "\n", verdict.AC},
		{"tokens: different value", problem.CheckerTokens, "6\n", "7\n", verdict.WA},
		{"tokens: split token", problem.CheckerTokens, "12\n", "1 2\n", verdict.WA},
		{"tokens: joined tokens", problem.CheckerTokens, "1 2\n", "12\n", verdict.WA},
		{"tokens: extra token", problem.CheckerTokens, "1 2\n", "1 2 3\n", verdict.WA},
		{"tokens: missing token", problem.CheckerTokens, "1 2\n", "1\n", verdict.WA},
		{"tokens: empty output", problem.CheckerTokens, "6\n", "", verdict.WA},
		{"tokens: wrong order", problem.CheckerTokens, "1 2\n", "2 1\n", verdict.WA},
		{"tokens: case matters", problem.CheckerTokens, "Yes\n", "yes\n", verdict.WA},
		{"exact: identical", problem.CheckerExact, "a b\nc\n", "a b\nc\n", verdict.AC},
		{"exact: trailing newline missing", problem.CheckerExact, "a\n", "a", verdict.AC},
		{"exact: trailing newline extra", problem.CheckerExact, "a", "a\n", verdict.AC},
		{"exact: two trailing newlines", problem.CheckerExact, "a\n", "a\n\n", verdict.WA},
		{"exact: inner spacing matters", problem.CheckerExact, "a b\n", "a  b\n", verdict.WA},
		{"exact: trailing space matters", problem.CheckerExact, "a\n", "a \n", verdict.WA},
		{"exact: crlf is different", problem.CheckerExact, "a\n", "a\r\n", verdict.WA},
		{"exact: empty vs newline", problem.CheckerExact, "", "\n", verdict.AC},
		{"exact: empty vs text", problem.CheckerExact, "a\n", "", verdict.WA},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Check(tc.mode, []byte(tc.expected), []byte(tc.actual))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("Check(%q, %q) = %q, want %q", tc.expected, tc.actual, got, tc.want)
			}
		})
	}
}

func TestCheckUnknownMode(t *testing.T) {
	got, err := Check("fuzzy", []byte("a"), []byte("a"))
	if err == nil {
		t.Fatal("expected an error for an unknown mode")
	}
	if got != "" {
		t.Errorf("verdict = %q, want none", got)
	}
}
