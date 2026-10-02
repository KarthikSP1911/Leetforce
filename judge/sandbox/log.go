package sandbox

import (
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

var (
	exitedRe     = regexp.MustCompile(`pid=\d+ \(.*\) exited with status: (\d+)`)
	signaledRe   = regexp.MustCompile(`pid=\d+ \(.*\) terminated with signal: \w+ \((\d+)\)`)
	timeLimitRe  = regexp.MustCompile(`pid=\d+ run time >= time limit`)
	sandboxErrRe = regexp.MustCompile(`^\[(E|F)\]`)
	// nsjail logs "Executing '<path>' for ..." only once the jail is fully
	// built and the program is about to start.
	executingRe = regexp.MustCompile(`^\[I\]\[[^\]]*\] Executing '`)
)

// termination is how nsjail says the sandboxed program ended.
type termination struct {
	started  bool           // the jail was built and the program was launched
	reported bool           // nsjail logged an exit or a signal
	exitCode int            // valid when signal == 0
	signal   syscall.Signal // non-zero if killed by a signal
	timedOut bool           // nsjail enforced its wall-time limit
	// failure holds nsjail's own error lines.
	failure []string
}

// parseLog reads nsjail's log. The last exit/signal line wins, so earlier text
// cannot override the real outcome.
func parseLog(log string) termination {
	var t termination
	for _, line := range strings.Split(log, "\n") {
		switch {
		case sandboxErrRe.MatchString(line):
			t.failure = append(t.failure, line)
		case executingRe.MatchString(line):
			t.started = true
		case timeLimitRe.MatchString(line):
			t.timedOut = true
		}
		if m := exitedRe.FindStringSubmatch(line); m != nil {
			code, _ := strconv.Atoi(m[1])
			t.reported, t.exitCode, t.signal = true, code, 0
		}
		if m := signaledRe.FindStringSubmatch(line); m != nil {
			sig, _ := strconv.Atoi(m[1])
			t.reported, t.exitCode, t.signal = true, -1, syscall.Signal(sig)
		}
	}
	return t
}
