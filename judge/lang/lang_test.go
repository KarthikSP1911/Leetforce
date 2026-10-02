package lang

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"leetforce/judge/problem"
)

// Every language a problem may list must have a definition, and no definition
// may be missing from the problem format's list.
func TestMatchesProblemLanguages(t *testing.T) {
	names := Names()
	slices.Sort(names)
	want := slices.Clone(problem.Languages)
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Errorf("lang.Names() = %v, problem.Languages = %v", names, want)
	}
}

func TestJavaRunArgv(t *testing.T) {
	l, _ := Get("java")
	argv := l.Run("/var/tmp/job", 256<<20)
	if argv[0] != javaHome+"/bin/java" {
		t.Errorf("java must be run by its real path, got %q", argv[0])
	}
	if !slices.Contains(argv, "-Xmx192m") || !slices.Contains(argv, "-XX:+ExitOnOutOfMemoryError") || l.OOMExitCode != 3 {
		t.Errorf("heap should be 64 MiB under the limit and OOM should exit with 3: %v, %d", argv, l.OOMExitCode)
	}
	if argv[len(argv)-1] != "Main" || !slices.Contains(argv, "/var/tmp/job/bin/main") {
		t.Errorf("classpath or main class missing: %v", argv)
	}
	if !slices.Contains(l.Binds, "/etc/java-21-openjdk") {
		t.Errorf("java needs the /etc configuration bind: %v", l.Binds)
	}
	for mb, want := range map[uint64]string{1: "-Xmx16m", 100: "-Xmx50m", 128: "-Xmx64m"} {
		if got := l.Run("/var/tmp/job", mb<<20); !slices.Contains(got, want) {
			t.Errorf("limit %d MiB: want %s in %v", mb, want, got)
		}
	}
}

func TestGet(t *testing.T) {
	for _, name := range []string{"python", "go", "cpp", "java"} {
		l, ok := Get(name)
		if !ok || l.Name != name {
			t.Fatalf("Get(%q) = %+v, %v", name, l, ok)
		}
	}
	if _, ok := Get("cobol"); ok {
		t.Error("Get(cobol) should fail")
	}
	if got := Names(); !slices.Contains(got, "go") || !slices.Contains(got, "python") {
		t.Errorf("Names() = %v", got)
	}
}

func TestPaths(t *testing.T) {
	l, _ := Get("go")
	if got := l.SourcePath("/var/tmp/job"); got != filepath.Join("/var/tmp/job", "src", "main.go") {
		t.Errorf("SourcePath = %q", got)
	}
	if got := ArtifactPath("/var/tmp/job"); got != filepath.Join("/var/tmp/job", "bin", "main") {
		t.Errorf("ArtifactPath = %q", got)
	}
}

// Every language needs limits the sandbox will accept, and an artifact
// language must be allowed to return its artifact.
func TestDefinitions(t *testing.T) {
	for _, name := range Names() {
		l, _ := Get(name)
		cl := l.CompileLimits
		if l.Source == "" || l.Run == nil || l.RunPIDs == 0 {
			t.Errorf("%s: incomplete definition", name)
		}
		if l.Compile != nil && (cl.Time <= 0 || cl.MemoryBytes == 0 || cl.PIDs == 0 || cl.MaxArtifactBytes <= 0) {
			t.Errorf("%s: bad compile limits %+v", name, cl)
		}
		if l.Compile != nil && len(l.Compile("/var/tmp/job")) == 0 {
			t.Errorf("%s: empty compile argv", name)
		}
		if argv := l.Run("/var/tmp/job", 128<<20); len(argv) == 0 || !strings.HasPrefix(argv[0], "/") {
			t.Errorf("%s: run argv must start with an absolute path: %v", name, argv)
		}
	}
	if l, _ := Get("go"); !l.Artifact {
		t.Error("go must return an artifact")
	}
	if l, _ := Get("python"); l.Artifact {
		t.Error("python has no artifact")
	}
}
