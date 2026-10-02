package lang

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	for _, name := range []string{"python", "go"} {
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
