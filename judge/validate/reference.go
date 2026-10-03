package validate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"leetforce/judge/engine"
	"leetforce/judge/problem"
	"leetforce/judge/verdict"
)

// ErrNoSandbox means the reference check cannot run on this host.
var ErrNoSandbox = errors.New("the reference-solution check needs the sandbox (Linux, root, nsjail, cgroup v2); run it on the dev host with sudo, or use -structure-only")

// Judger is the part of *engine.Engine the reference check uses.
type Judger interface {
	Judge(ctx context.Context, p *problem.Problem, language string, source []byte, opts engine.Options) (*engine.Report, error)
}

// Solution is one reference file: solutions/<language>/<verdict>.<ext>.
type Solution struct {
	Language string
	Path     string
	Expected verdict.Verdict
}

var extLang = map[string]string{"py": "python", "cpp": "cpp", "java": "java", "go": "go"}

var expectedByName = map[string]verdict.Verdict{
	"ac": verdict.AC, "wa": verdict.WA, "tle": verdict.TLE, "mle": verdict.MLE,
	"re": verdict.RE, "ce": verdict.CE, "ole": verdict.OLE,
}

// Solutions lists the reference solutions under dir/solutions in a stable
// order. The file name before the extension is the expected verdict
// (ac.py, wa.cpp, tle.go ...). A file that does not fit is returned as an
// issue instead of being skipped silently.
func Solutions(dir string) ([]Solution, []Issue) {
	var sols []Solution
	var issues []Issue
	root := filepath.Join(dir, "solutions")
	langs, err := os.ReadDir(root)
	if err != nil {
		return nil, []Issue{{Error, fmt.Sprintf("solutions/: %v", err)}}
	}
	for _, l := range langs {
		if !l.IsDir() || !contains(problem.Languages, l.Name()) {
			issues = append(issues, Issue{Error, fmt.Sprintf("solutions/%s: expected one directory per language (%s)", l.Name(), strings.Join(problem.Languages, ", "))})
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, l.Name()))
		if err != nil {
			issues = append(issues, Issue{Error, fmt.Sprintf("solutions/%s: %v", l.Name(), err)})
			continue
		}
		for _, f := range files {
			stem, ext, _ := strings.Cut(f.Name(), ".")
			want, okName := expectedByName[strings.ToLower(stem)]
			if f.IsDir() || !okName || extLang[ext] != l.Name() {
				issues = append(issues, Issue{Error, fmt.Sprintf("solutions/%s/%s: name must be <verdict>.<ext>, verdict one of ac wa tle mle re ce ole", l.Name(), f.Name())})
				continue
			}
			sols = append(sols, Solution{Language: l.Name(), Path: filepath.Join(root, l.Name(), f.Name()), Expected: want})
		}
	}
	sort.Slice(sols, func(i, j int) bool { return sols[i].Path < sols[j].Path })
	return sols, issues
}

// Reference judges every reference solution of p and returns an issue for each
// one whose verdict is not the one its name promises. An ac solution must pass
// all tests. At least one ac solution is required, and a language that has
// solutions but no ac one is an error. Host failures (an error from the
// engine) are returned as the error, not as an issue.
func Reference(ctx context.Context, j Judger, p *problem.Problem) ([]Issue, error) {
	sols, issues := Solutions(p.Dir)
	acByLang := map[string]bool{}
	langs := map[string]bool{}
	for _, s := range sols {
		langs[s.Language] = true
		if s.Expected == verdict.AC {
			acByLang[s.Language] = true
		}
	}
	if len(acByLang) == 0 {
		issues = append(issues, Issue{Error, "no ac.<ext> reference solution in any language"})
	}
	for _, l := range problem.Languages {
		if langs[l] && !acByLang[l] {
			issues = append(issues, Issue{Error, fmt.Sprintf("solutions/%s has no ac solution", l)})
		}
	}
	for _, s := range sols {
		src, err := os.ReadFile(s.Path) //nolint:gosec // operator-chosen dir
		if err != nil {
			return issues, fmt.Errorf("read %s: %w", s.Path, err)
		}
		rep, err := j.Judge(ctx, p, s.Language, src, engine.Options{ContinueOnFail: s.Expected == verdict.AC})
		if err != nil {
			return issues, fmt.Errorf("judge %s: %w", s.Path, err)
		}
		rel, _ := filepath.Rel(p.Dir, s.Path)
		rel = filepath.ToSlash(rel)
		got := rep.Overall.Verdict
		switch {
		case got != s.Expected:
			msg := fmt.Sprintf("%s: expected %s, got %s", rel, s.Expected, got)
			if rep.Overall.Failed != "" && got != verdict.CE {
				msg += " (first failing test " + rep.Overall.Failed + ")"
			}
			issues = append(issues, Issue{Error, msg})
		case s.Expected == verdict.AC:
			for _, c := range rep.Cases {
				if c.Verdict != verdict.AC {
					issues = append(issues, Issue{Error, fmt.Sprintf("%s: test %s is %s", rel, c.Name, c.Verdict)})
				}
			}
		}
	}
	return issues, nil
}
