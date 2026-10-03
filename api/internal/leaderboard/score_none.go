//go:build !leaderboard_stub

package leaderboard

// ContractScorer is nil in a build without the Phase 14 contract: the
// standings endpoint then answers 503. Phase 16 deletes this file.
func ContractScorer() Scorer { return nil }
