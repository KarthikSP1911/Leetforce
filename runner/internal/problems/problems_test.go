package problems

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"leetforce/judge/problem"
	"leetforce/storage"
)

const sampleDir = "../../../problems/sample-sum"

// fakeBundles serves one bundle and counts downloads.
type fakeBundles struct {
	data      []byte
	statErr   error
	noHash    bool
	downloads int
}

func (f *fakeBundles) Stat(context.Context, string, string) (string, error) {
	if f.statErr != nil {
		return "", f.statErr
	}
	if f.noHash {
		return "", nil
	}
	return storage.Hash(f.data), nil
}

func (f *fakeBundles) GetBundle(context.Context, string, string) ([]byte, error) {
	f.downloads++
	return f.data, nil
}

func sampleBundle(t *testing.T) ([]byte, string) {
	t.Helper()
	p, err := problem.Load(sampleDir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := problem.Pack(sampleDir)
	if err != nil {
		t.Fatal(err)
	}
	return data, p.TestSetVer
}

func TestS3FetchesOnceThenUsesCache(t *testing.T) {
	data, ver := sampleBundle(t)
	f := &fakeBundles{data: data}
	s := S3{Store: f, Cache: t.TempDir()}
	for range 3 {
		p, err := s.Load(context.Background(), "sample-sum", ver)
		if err != nil {
			t.Fatal(err)
		}
		if p.TestSetVer != ver || len(p.Tests) != 5 {
			t.Fatalf("loaded %s with %d tests", p.TestSetVer, len(p.Tests))
		}
	}
	if f.downloads != 1 {
		t.Errorf("downloads = %d, want 1 (later loads use the cache)", f.downloads)
	}
}

func TestS3WithoutHashStillWorks(t *testing.T) {
	data, ver := sampleBundle(t)
	f := &fakeBundles{data: data, noHash: true}
	s := S3{Store: f, Cache: t.TempDir()}
	for range 2 {
		if _, err := s.Load(context.Background(), "sample-sum", ver); err != nil {
			t.Fatal(err)
		}
	}
	if f.downloads != 2 {
		t.Errorf("downloads = %d, want 2 (no hash means no cache check)", f.downloads)
	}
}

func TestS3Errors(t *testing.T) {
	data, ver := sampleBundle(t)
	other := "ts-ffffffffffffffff"
	tests := []struct {
		name      string
		slug, ver string
		store     *fakeBundles
		permanent bool
		wantIs    error
	}{
		{"bad slug", "../x", ver, &fakeBundles{data: data}, true, nil},
		{"no version", "sample-sum", "", &fakeBundles{data: data}, true, nil},
		{"version mismatch", "sample-sum", other, &fakeBundles{data: data}, true, nil},
		{"corrupt bundle", "sample-sum", ver, &fakeBundles{data: []byte("junk")}, true, nil},
		{"wrong problem", "other-problem", ver, &fakeBundles{data: data}, true, nil},
		{"not uploaded yet", "sample-sum", ver, &fakeBundles{statErr: storage.ErrNotFound}, false, storage.ErrNotFound},
		{"storage down", "sample-sum", ver, &fakeBundles{statErr: errors.New("connection refused")}, false, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := S3{Store: tc.store, Cache: t.TempDir()}.Load(context.Background(), tc.slug, tc.ver)
			if err == nil {
				t.Fatal("Load succeeded")
			}
			if got := errors.Is(err, ErrPermanent); got != tc.permanent {
				t.Errorf("permanent = %v, want %v (err: %v)", got, tc.permanent, err)
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("err = %v, want %v", err, tc.wantIs)
			}
		})
	}
}

func TestS3RefetchesWhenBundleChanges(t *testing.T) {
	data, ver := sampleBundle(t)
	f := &fakeBundles{data: data}
	s := S3{Store: f, Cache: t.TempDir()}
	if _, err := s.Load(context.Background(), "sample-sum", ver); err != nil {
		t.Fatal(err)
	}
	// Same tests, new limits: same version, different bundle bytes.
	src := t.TempDir()
	if err := problem.Unpack(data, src); err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(src, "problem.yaml")
	raw, err := os.ReadFile(yamlPath) //nolint:gosec // path is under t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("# limits tuned\n")...)
	if err := os.WriteFile(yamlPath, raw, 0o600); err != nil { //nolint:gosec // path is under t.TempDir()
		t.Fatal(err)
	}
	f.data, err = problem.Pack(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(context.Background(), "sample-sum", ver); err != nil {
		t.Fatal(err)
	}
	if f.downloads != 2 {
		t.Errorf("downloads = %d, want 2 after the bundle changed", f.downloads)
	}
}

func TestDirRejectsBadSlug(t *testing.T) {
	d := Dir{Root: "../../../problems"}
	if _, err := d.Load(context.Background(), "../etc", ""); !errors.Is(err, ErrPermanent) {
		t.Errorf("bad slug: %v", err)
	}
	if _, err := d.Load(context.Background(), "no-such-problem", ""); !errors.Is(err, ErrPermanent) {
		t.Errorf("missing problem: %v", err)
	}
	if p, err := d.Load(context.Background(), "sample-sum", ""); err != nil || len(p.Tests) != 5 {
		t.Errorf("sample-sum: %v", err)
	}
}
