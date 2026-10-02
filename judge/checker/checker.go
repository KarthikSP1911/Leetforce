// Package checker compares a program's output with the expected output. It
// runs on the host, outside the sandbox, on bytes the sandbox returned; the
// program never sees the expected output and cannot influence the comparison
// other than through its own stdout.
package checker

import (
	"bytes"
	"fmt"

	"leetforce/judge/problem"
	"leetforce/judge/verdict"
)

// Check compares actual with expected using the named checker mode and
// returns AC or WA. An unknown mode is an error, not a verdict.
//
//   - problem.CheckerExact: byte for byte, except that one trailing "\n" on
//     either side is ignored.
//   - problem.CheckerTokens: the outputs must have the same whitespace-separated
//     tokens in the same order; spacing, blank lines, "\r\n" and trailing
//     whitespace do not matter.
func Check(mode string, expected, actual []byte) (verdict.Verdict, error) {
	var ok bool
	switch mode {
	case problem.CheckerExact:
		ok = bytes.Equal(trimOneNewline(expected), trimOneNewline(actual))
	case problem.CheckerTokens:
		ok = sameTokens(expected, actual)
	default:
		return "", fmt.Errorf("checker: unknown mode %q", mode)
	}
	if ok {
		return verdict.AC, nil
	}
	return verdict.WA, nil
}

func trimOneNewline(b []byte) []byte {
	return bytes.TrimSuffix(b, []byte("\n"))
}

func sameTokens(a, b []byte) bool {
	fa, fb := bytes.Fields(a), bytes.Fields(b)
	if len(fa) != len(fb) {
		return false
	}
	for i := range fa {
		if !bytes.Equal(fa[i], fb[i]) {
			return false
		}
	}
	return true
}
