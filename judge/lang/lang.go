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
	// Binds are extra host paths mounted read-only in both the compile and run
	// sandboxes (for example the JDK configuration the Debian package keeps in /etc).
	Binds []string
}

// CompileLimits bound the compile sandbox.
type CompileLimits struct {
	Time        time.Duration // wall and CPU
	MemoryBytes uint64
	PIDs        uint64
	TmpfsBytes  uint64 // writable /tmp; counts against the memory limit
	// MaxFileBytes is the largest single file the compiler may write (build
	// archives can be large); zero keeps the sandbox default.
	MaxFileBytes uint64
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
		CompileEnv: []string{sysPath, "GOROOT=/usr/local/go", "GOCACHE=/tmp/gocache", "GOTELEMETRY=off", "GOTOOLCHAIN=local", "CGO_ENABLED=0", "GOFLAGS=-p=1"},
		CompileLimits: CompileLimits{
			Time: 60 * time.Second, MemoryBytes: 400 << 20, PIDs: 128, TmpfsBytes: 96 << 20, MaxFileBytes: 64 << 20,
			MaxArtifactBytes: 32 << 20,
		},
		Run: func(dir string, _ uint64) []string {
			return []string{ArtifactPath(dir)}
		},
		RunEnv:  []string{"GOMAXPROCS=2"},
		RunPIDs: 32,
	},
	"cpp": {
		Name:     "cpp",
		Source:   "main.cpp",
		Artifact: true,
		// Static, so the run needs nothing from the host but the kernel.
		Compile: func(dir string) []string {
			script := fmt.Sprintf("g++ -O2 -pipe -std=c++17 -static -o /tmp/main %q && cat /tmp/main >&4", filepath.Join(dir, "src", "main.cpp"))
			return []string{"/bin/sh", "-c", script}
		},
		CompileEnv: []string{sysPath},
		CompileLimits: CompileLimits{
			Time: 30 * time.Second, MemoryBytes: 400 << 20, PIDs: 64, TmpfsBytes: 64 << 20, MaxFileBytes: 64 << 20,
			MaxArtifactBytes: 32 << 20,
		},
		Run: func(dir string, _ uint64) []string {
			return []string{ArtifactPath(dir)}
		},
		RunPIDs: 16,
	},
	"java": {
		Name:     "java",
		Source:   "Main.java",
		Artifact: true,
		// Compile to a single jar so the artifact is one file. The -J flags
		// size the compiler's own JVM; JAVA_TOOL_OPTIONS is not used because it
		// would print a notice into the compiler output.
		Compile: func(dir string) []string {
			script := fmt.Sprintf(javaHome+"/bin/javac -J-Xmx192m -J-XX:+UseSerialGC -J-XX:-UsePerfData -d /tmp/out %q"+
				" && "+javaHome+"/bin/jar -J-Xmx64m -J-XX:+UseSerialGC -J-XX:-UsePerfData cfe /tmp/main.jar Main -C /tmp/out ."+
				" && cat /tmp/main.jar >&4", filepath.Join(dir, "src", "Main.java"))
			return []string{"/bin/sh", "-c", script}
		},
		CompileEnv: []string{sysPath, "JAVA_HOME=" + javaHome, javaLibPath},
		CompileLimits: CompileLimits{
			Time: 45 * time.Second, MemoryBytes: 420 << 20, PIDs: 128, TmpfsBytes: 32 << 20, MaxFileBytes: 32 << 20,
			MaxArtifactBytes: 8 << 20,
		},
		Run: func(dir string, memoryBytes uint64) []string {
			// The heap may grow to twice the memory limit. The kernel's memory
			// cgroup, not the JVM, decides when a program has used too much, so
			// an allocation loop ends in an OOM kill (MLE, measured by the host)
			// rather than a catchable OutOfMemoryError that would look like RE.
			heapMB := max(memoryBytes>>20, 16) * 2
			return []string{
				javaHome + "/bin/java",
				fmt.Sprintf("-Xmx%dm", heapMB), "-Xms16m", "-Xss64m",
				"-XX:+UseSerialGC", "-XX:-UsePerfData", "-XX:TieredStopAtLevel=1",
				"-cp", ArtifactPath(dir), "Main",
			}
		},
		// The Debian JDK keeps its configuration in /etc and links to it.
		RunEnv:  []string{javaLibPath},
		Binds:   []string{"/etc/java-21-openjdk"},
		RunPIDs: 64,
	},
}

// javaHome is where the Debian openjdk-21-jdk-headless package installs the
// JDK. /usr/bin/java goes through /etc/alternatives, which the sandbox does
// not have, so the real path is used.
const javaHome = "/usr/lib/jvm/java-21-openjdk-amd64"

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

// javaLibPath lets the JDK tools find their shared libraries. They normally
// locate them through an $ORIGIN rpath, which glibc resolves via /proc/self/exe,
// and the sandbox has no /proc.
const javaLibPath = "LD_LIBRARY_PATH=" + javaHome + "/lib:" + javaHome + "/lib/server"
