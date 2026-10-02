package catalog

import (
	"bytes"
	"context"
	"testing"

	"leetforce/judge/problem"
)

// The repository's own problems directory must load, and only the tests the
// problem marks as samples may come out of Samples.
func TestLoadRepoProblems(t *testing.T) {
	c, err := Load("../../../problems")
	if err != nil {
		t.Fatal(err)
	}
	ps := c.Problems()
	if len(ps) == 0 {
		t.Fatal("no problems loaded")
	}
	for _, p := range ps {
		want := 0
		for _, tc := range p.Tests {
			if tc.Sample {
				want++
			}
		}
		if len(p.Tests) <= want {
			t.Fatalf("%s: expected hidden tests besides the samples", p.Spec.Slug)
		}
		got := c.Samples(p.Spec.Slug)
		if len(got) != want {
			t.Fatalf("%s: Samples returned %d tests, want only the %d samples (of %d tests)", p.Spec.Slug, len(got), want, len(p.Tests))
		}
	}
	if c.Samples("does-not-exist") != nil {
		t.Fatal("unknown slug must have no samples")
	}
}

func TestLoadRejectsMissingDir(t *testing.T) {
	if _, err := Load("no/such/dir"); err == nil {
		t.Fatal("want an error for a missing directory")
	}
}

// fakeBundles records what Publish stores.
type fakeBundles struct{ got map[string][]byte }

func (f *fakeBundles) PutBundle(_ context.Context, slug, version string, data []byte) (bool, error) {
	if f.got == nil {
		f.got = map[string][]byte{}
	}
	k := slug + "/" + version
	if bytes.Equal(f.got[k], data) {
		return false, nil
	}
	f.got[k] = data
	return true, nil
}

// Publish stores one bundle per problem under its test-set version, the bundle
// unpacks to the same version, and a second Publish writes nothing.
func TestPublish(t *testing.T) {
	c, err := Load("../../../problems")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeBundles{}
	n, err := c.Publish(context.Background(), f)
	if err != nil || n != len(c.Problems()) {
		t.Fatalf("first Publish = %d, %v; want %d", n, err, len(c.Problems()))
	}
	for _, p := range c.Problems() {
		data := f.got[p.Spec.Slug+"/"+p.TestSetVer]
		if data == nil {
			t.Fatalf("%s: no bundle under version %s", p.Spec.Slug, p.TestSetVer)
		}
		dir := t.TempDir()
		if err := problem.Unpack(data, dir); err != nil {
			t.Fatal(err)
		}
		back, err := problem.Load(dir)
		if err != nil || back.TestSetVer != p.TestSetVer {
			t.Fatalf("%s: bundle loads as %v, %v", p.Spec.Slug, back, err)
		}
	}
	if n, err := c.Publish(context.Background(), f); err != nil || n != 0 {
		t.Fatalf("second Publish = %d, %v; want 0 writes", n, err)
	}
}
