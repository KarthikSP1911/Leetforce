package problem

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func addFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadContent(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantErr     bool
		wantStmt    string
		wantStarter map[string]string
	}{
		{"none", nil, false, "", map[string]string{}},
		{"statement only", map[string]string{"statement.md": "# Hi\n"}, false, "# Hi\n", map[string]string{}},
		{"all starters", map[string]string{
			"starters/python.py": "py", "starters/cpp.cpp": "cc",
			"starters/java.java": "jv", "starters/go.go": "gg",
		}, false, "", map[string]string{"python": "py", "cpp": "cc", "java": "jv", "go": "gg"}},
		{"some starters", map[string]string{"starters/go.go": "gg"}, false, "", map[string]string{"go": "gg"}},
		{"unknown starter file", map[string]string{"starters/rust.rs": "x"}, true, "", nil},
		{"starter subdirectory", map[string]string{"starters/python.py/x": "x"}, true, "", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeProblem(t, validYAML, okFiles)
			addFiles(t, dir, tc.files)
			p, err := Load(dir)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if p.Statement != tc.wantStmt || !reflect.DeepEqual(p.Starters, tc.wantStarter) {
				t.Errorf("statement %q starters %v", p.Statement, p.Starters)
			}
		})
	}
}

// Editing the statement or starters must not change the test-set version,
// or every statement typo fix would orphan the submissions judged before it.
func TestVersionIgnoresStatementAndStarters(t *testing.T) {
	dir := writeProblem(t, validYAML, okFiles)
	before, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	addFiles(t, dir, map[string]string{"statement.md": "new text", "starters/python.py": "pass"})
	after, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before.TestSetVer != after.TestSetVer {
		t.Fatalf("version changed from %s to %s", before.TestSetVer, after.TestSetVer)
	}
	addFiles(t, dir, map[string]string{"tests/02.out": "3\n"})
	changed, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if changed.TestSetVer == after.TestSetVer {
		t.Fatal("version must change when a test changes")
	}
}

// Statement and starters must stay out of the runner bundle.
func TestPackExcludesStatementAndStarters(t *testing.T) {
	dir := writeProblem(t, validYAML, okFiles)
	plain, err := Pack(dir)
	if err != nil {
		t.Fatal(err)
	}
	addFiles(t, dir, map[string]string{"statement.md": "x", "starters/go.go": "y"})
	with, err := Pack(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plain, with) {
		t.Fatal("bundle changed when statement/starters were added")
	}
}

// Every committed problem except the Phase 2 fixture loads with a statement,
// a starter per language and at least six tests.
func TestRepoProblemsHaveContent(t *testing.T) {
	root := filepath.Join("..", "..", "problems")
	ys, err := filepath.Glob(filepath.Join(root, "*", "problem.yaml"))
	if err != nil || len(ys) == 0 {
		t.Fatalf("no problems found: %v", err)
	}
	for _, y := range ys {
		dir := filepath.Dir(y)
		slug := filepath.Base(dir)
		if slug == "sample-sum" {
			continue
		}
		t.Run(slug, func(t *testing.T) {
			p, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(p.Statement) == "" {
				t.Error("missing statement.md")
			}
			for _, l := range Languages {
				if p.Starters[l] == "" {
					t.Errorf("missing starter for %s", l)
				}
			}
			if len(p.Tests) < 6 {
				t.Errorf("tests = %d, want >= 6", len(p.Tests))
			}
		})
	}
}
