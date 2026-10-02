package problem

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// Version returns the test-set version: a short hash over the checker and
// every test's name, input and expected output (tests must be in name order,
// as Load returns them). Submissions record it so they can be rejudged after a
// test fix. Limits and titles are not part of it; they do not change what a
// correct answer is. Each field is length-prefixed so that moving bytes
// between fields changes the hash.
func Version(checker string, tests []Test) string {
	h := sha256.New()
	write := func(b []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		h.Write(n[:])
		h.Write(b)
	}
	write([]byte(checker))
	for _, t := range tests {
		write([]byte(t.Name))
		write(t.Input)
		write(t.Expected)
	}
	return "ts-" + hex.EncodeToString(h.Sum(nil))[:16]
}
