// Package lang describes how each supported language is checked, compiled and
// run inside the sandbox. A language is data: file names, argv and limits. The
// engine does the running.
//
// Layout of a job directory on the host, bind-mounted read-only at the same
// path inside every sandboxed run:
//
//	<dir>/src/<Language.Source>   the user's source, written by the host
//	<dir>/bin/main                the built artifact, if the language has one
//
// A compile step runs in its own sandbox with only the source visible. It
// hands the artifact back by writing it to the result descriptor (fd 4), so
// the host stores it and no sandbox ever gets a writable host directory.
package lang

import (
	"fmt"
	"path/filepath"
	"time"
)

// Language is one supported language.
type Language struct {
	Name   string // matches the names in problem.Languages
	Source string // source file name

	// Compile returns the argv of the compile step for a job directory, or nil
	// if the language has none. Any non-zero exit, signal or limit hit counts
	// as a compile error.
	Compile    func(dir string) []string
	CompileEnv []string
	// Artifact is true when the compile step must write the single built file
	// to fd 4 for the host to store as <dir>/bin/main.
	Artifact bool
	// CompileLimits are the resources for the compile sandbox.
	CompileLimits CompileLimits

	// Run returns the argv that runs the program for a job directory; memory is
	// the memory limit in bytes so runtimes with a heap setting can stay inside it.
	Run    func(dir string, memoryBytes uint64) []string
	RunEnv []string
	// RunPIDs is the process and thread cap for a run. Runtimes that start
	// helper threads at launch need more than a native program.
	RunPIDs uint64
}

// CompileLimits bound the compile sandbox.
type CompileLimits struct {
	Time        time.Duration // wall and CPU
	MemoryBytes uint64
	PIDs        uint64
	TmpfsBytes  uint64 // writable /tmp; counts against the memory limit
	// MaxArtifactBytes caps what the compile step may write to fd 4.
	MaxArtifactBytes int64
}

// SourcePath is where the user's source is stored for a job directory.
func (l Language) SourcePath(dir string) string { return filepath.Join(dir, "src", l.Source) }

// ArtifactPath is where the built artifact is stored for a job directory.
func ArtifactPath(dir string) string { return filepath.Join(dir, "bin", "main") }

const sysPath = "PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"

var languages = map[string]Language{
	"python": {
		Name:   "python",
		Source: "main.py",
		// Compile only to find syntax errors: nothing is written, so the
		// read-only source directory is fine.
		Compile: func(dir string) []string {
			src := filepath.Join(dir, "src", "main.py")
			return []string{
				"/usr/bin/python3", "-c",
				"import sys; compile(open(sys.argv[1]).read(), 'main.py', 'exec')", src,
			}
		},
		CompileEnv: []string{"PYTHONDONTWRITEBYTECODE=1"},
		CompileLimits: CompileLimits{
			Time: 10 * time.Second, MemoryBytes: 256 << 20, PIDs: 32, TmpfsBytes: 8 << 20,
			MaxArtifactBytes: 1,
		},
		Run: func(dir string, _ uint64) []string {
			return []string{"/usr/bin/python3", filepath.Join(dir, "src", "main.py")}
		},
		RunEnv:  []string{"PYTHONDONTWRITEBYTECODE=1"},
		RunPIDs: 32,
	},
	"go": {
		Name:     "go",
		Source:   "main.go",
		Artifact: true,
		Compile: func(dir string) []string {
			script := fmt.Sprintf("go build -o /tmp/main %q && cat /tmp/main >&4", filepath.Join(dir, "src", "main.go"))
			return []string{"/bin/sh", "-c", script}
		},
		// GOCACHE is in the sandbox's tmpfs, so every compile starts cold
		// (about 11 s and 260 MB on the dev host; see ADR 0006).
		CompileEnv: []string{sysPath, "GOCACHE=/tmp/gocache", "GOTOOLCHAIN=local", "CGO_ENABLED=0", "GOFLAGS=-p=1"},
		CompileLimits: CompileLimits{
			Time: 60 * time.Second, MemoryBytes: 400 << 20, PIDs: 128, TmpfsBytes: 96 << 20,
			MaxArtifactBytes: 32 << 20,
		},
		Run: func(dir string, _ uint64) []string {
			return []string{ArtifactPath(dir)}
		},
		RunEnv:  []string{"GOMAXPROCS=2"},
		RunPIDs: 32,
	},
}

// Get returns the language with the given name.
func Get(name string) (Language, bool) {
	l, ok := languages[name]
	return l, ok
}

// Names lists the languages defined so far.
func Names() []string {
	out := make([]string, 0, len(languages))
	for n := range languages {
		out = append(out, n)
	}
	return out
}
