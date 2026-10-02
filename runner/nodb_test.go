package runner_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Runners never connect to the database (CLAUDE.md, security rules). This
// enforces it at build level: no database package may appear in the runner's
// go.mod or anywhere in the dependency graph of the runner binary, including
// what it pulls in through the judge and queue modules.
var forbidden = []string{
	"database/sql",
	"github.com/jackc/pgx",
	"github.com/lib/pq",
	"github.com/gin-gonic/gin", // the API's framework; the runner talks to the API only through Redis
	"gorm.io",
	"github.com/jmoiron/sqlx",
}

func TestRunnerHasNoDatabaseDependency(t *testing.T) {
	mod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range forbidden {
		if strings.Contains(string(mod), f) {
			t.Errorf("runner/go.mod mentions %q", f)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "go", "list", "-deps", "./...").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	deps := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(deps) < 20 {
		t.Fatalf("go list returned only %d packages; the check would prove nothing:\n%s", len(deps), out)
	}
	for _, dep := range deps {
		for _, f := range forbidden {
			if dep == f || strings.HasPrefix(dep, f+"/") || (strings.HasSuffix(f, "/pgx") && strings.HasPrefix(dep, f)) {
				t.Errorf("runner depends on %q", dep)
			}
		}
	}
}
