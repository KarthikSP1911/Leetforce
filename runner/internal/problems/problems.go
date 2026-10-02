// Package problems gives the runner the problem (spec and every test) to judge
// a job against. A job names a problem and the test-set version its
// submission was accepted against; the S3 source fetches exactly that bundle
// and refuses anything whose recomputed version differs, so a runner never
// judges against tests the submission was not stamped with.
package problems

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"leetforce/judge/problem"
	"leetforce/storage"
)

// ErrPermanent marks failures no retry can fix (a bad name, a corrupt or
// mismatched bundle). Everything else (storage unreachable, bundle not yet
// uploaded) may succeed on a later delivery.
var ErrPermanent = errors.New("problem cannot be loaded")

var (
	slugRe    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	versionRe = regexp.MustCompile(`^ts-[0-9a-f]{16}$`)
)

// Source loads a problem for a job.
type Source interface {
	Load(ctx context.Context, slug, version string) (*problem.Problem, error)
}

// Dir reads problems from a local directory (problems/<slug>). It ignores the
// version: use it for development and tests without object storage.
type Dir struct{ Root string }

// Load implements Source.
func (d Dir) Load(_ context.Context, slug, _ string) (*problem.Problem, error) {
	if !slugRe.MatchString(slug) {
		return nil, fmt.Errorf("%w: invalid problem slug %q", ErrPermanent, slug)
	}
	p, err := problem.Load(filepath.Join(d.Root, slug))
	if err != nil {
		return nil, fmt.Errorf("%w: load problem %q: %w", ErrPermanent, slug, err)
	}
	return p, nil
}

// Bundles is the part of *storage.Store the S3 source uses.
type Bundles interface {
	Stat(ctx context.Context, slug, version string) (string, error)
	GetBundle(ctx context.Context, slug, version string) ([]byte, error)
}

// S3 loads problems from object storage and keeps unpacked copies under Cache,
// one directory per bundle hash.
type S3 struct {
	Store Bundles
	Cache string
}

// Load implements Source. The cache is checked against the bundle hash the
// store reports, so a bundle replaced under the same version (new limits) is
// fetched again; an unchanged one costs a single metadata request.
func (s S3) Load(ctx context.Context, slug, version string) (*problem.Problem, error) {
	if !slugRe.MatchString(slug) {
		return nil, fmt.Errorf("%w: invalid problem slug %q", ErrPermanent, slug)
	}
	if !versionRe.MatchString(version) {
		return nil, fmt.Errorf("%w: job has no valid test-set version (%q)", ErrPermanent, version)
	}
	sum, err := s.Store.Stat(ctx, slug, version)
	if err != nil {
		return nil, fmt.Errorf("stat bundle %s %s: %w", slug, version, err)
	}
	if p, ok := s.cached(slug, version, sum); ok {
		return p, nil
	}
	data, err := s.Store.GetBundle(ctx, slug, version)
	if err != nil {
		return nil, fmt.Errorf("fetch bundle %s %s: %w", slug, version, err)
	}
	return s.install(slug, version, storage.Hash(data), data)
}

func (s S3) dir(slug, version, sum string) string {
	return filepath.Join(s.Cache, slug+"-"+version+"-"+sum[:16])
}

// cached returns the unpacked bundle if it is already on disk and still loads
// to the requested version. An empty or short sum (the object carries no hash)
// never hits the cache.
func (s S3) cached(slug, version, sum string) (*problem.Problem, bool) {
	if len(sum) < 16 {
		return nil, false
	}
	p, err := problem.Load(s.dir(slug, version, sum))
	if err != nil || p.TestSetVer != version {
		return nil, false
	}
	return p, true
}

// install unpacks into a temporary directory, checks the problem and its
// version, then renames it into place so a half-written cache entry is never
// visible to a concurrent load.
func (s S3) install(slug, version, sum string, data []byte) (*problem.Problem, error) {
	final := s.dir(slug, version, sum)
	if p, ok := s.cached(slug, version, sum); ok {
		return p, nil
	}
	if err := os.MkdirAll(s.Cache, 0o750); err != nil {
		return nil, fmt.Errorf("create problem cache: %w", err)
	}
	tmp, err := os.MkdirTemp(s.Cache, ".unpack-*")
	if err != nil {
		return nil, fmt.Errorf("create problem cache entry: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := problem.Unpack(data, tmp); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	p, err := problem.Load(tmp)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	if p.Spec.Slug != slug {
		return nil, fmt.Errorf("%w: bundle for %q holds problem %q", ErrPermanent, slug, p.Spec.Slug)
	}
	if p.TestSetVer != version {
		return nil, fmt.Errorf("%w: bundle %s %s holds test set %s", ErrPermanent, slug, version, p.TestSetVer)
	}
	_ = os.RemoveAll(final) // a stale or half-valid entry
	if err := os.Rename(tmp, final); err != nil {
		return nil, fmt.Errorf("install problem cache entry: %w", err)
	}
	p.Dir = final
	return p, nil
}
