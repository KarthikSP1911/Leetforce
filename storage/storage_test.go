package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

func TestKeyRejectsBadNames(t *testing.T) {
	tests := []struct {
		slug, version string
		ok            bool
	}{
		{"sample-sum", "ts-0123456789abcdef", true},
		{"../x", "ts-0123456789abcdef", false},
		{"Sample", "ts-0123456789abcdef", false},
		{"sample-sum", "latest", false},
		{"sample-sum", "ts-0123456789abcdef/../x", false},
		{"", "ts-0123456789abcdef", false},
	}
	for _, tc := range tests {
		_, err := key(tc.slug, tc.version)
		if (err == nil) != tc.ok {
			t.Errorf("key(%q, %q) error = %v, want ok=%v", tc.slug, tc.version, err, tc.ok)
		}
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("LEETFORCE_S3_ENDPOINT", "")
	if _, ok, err := ConfigFromEnv(); ok || err != nil {
		t.Fatalf("no endpoint: ok=%v err=%v, want not configured", ok, err)
	}
	t.Setenv("LEETFORCE_S3_ENDPOINT", "127.0.0.1:9000")
	t.Setenv("LEETFORCE_S3_ACCESS_KEY", "")
	t.Setenv("LEETFORCE_S3_SECRET_KEY", "")
	if _, ok, err := ConfigFromEnv(); err != nil || !ok {
		t.Fatalf("endpoint without keys must select the IAM role: ok=%v err=%v", ok, err)
	}
	t.Setenv("LEETFORCE_S3_ACCESS_KEY", "a")
	if _, _, err := ConfigFromEnv(); err == nil {
		t.Fatal("only one key set must be an error")
	}
	t.Setenv("LEETFORCE_S3_ACCESS_KEY", "a")
	t.Setenv("LEETFORCE_S3_SECRET_KEY", "b")
	cfg, ok, err := ConfigFromEnv()
	if err != nil || !ok || cfg.Bucket != "leetforce-problems" || cfg.UseTLS {
		t.Fatalf("cfg=%+v ok=%v err=%v", cfg, ok, err)
	}
}

// TestRoundTrip runs against a real S3 server (the dev host's RustFS) when
// LEETFORCE_S3_ENDPOINT is set; otherwise it is skipped.
func TestRoundTrip(t *testing.T) {
	cfg, ok, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Skip("LEETFORCE_S3_ENDPOINT not set")
	}
	cfg.Bucket = fmt.Sprintf("lftest-%d", time.Now().UnixNano())
	s, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Ping(ctx); err == nil {
		t.Fatal("Ping succeeded for a bucket that does not exist")
	}
	if err := s.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.c.RemoveBucket(ctx, cfg.Bucket) })
	if err := s.EnsureBucket(ctx); err != nil {
		t.Fatalf("second EnsureBucket: %v", err)
	}
	if err := s.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	const slug, ver = "demo-problem", "ts-0123456789abcdef"
	if _, err := s.GetBundle(ctx, slug, ver); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetBundle of a missing bundle: %v, want ErrNotFound", err)
	}
	if _, err := s.Stat(ctx, slug, ver); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Stat of a missing bundle: %v, want ErrNotFound", err)
	}

	v1 := []byte("bundle one")
	if wrote, err := s.PutBundle(ctx, slug, ver, v1); err != nil || !wrote {
		t.Fatalf("first put: wrote=%v err=%v", wrote, err)
	}
	t.Cleanup(func() {
		_ = s.c.RemoveObject(ctx, cfg.Bucket, "problems/"+slug+"/"+ver+".tar.gz", minio.RemoveObjectOptions{})
	})
	if wrote, err := s.PutBundle(ctx, slug, ver, v1); err != nil || wrote {
		t.Fatalf("identical put must be a no-op: wrote=%v err=%v", wrote, err)
	}
	got, err := s.GetBundle(ctx, slug, ver)
	if err != nil || !bytes.Equal(got, v1) {
		t.Fatalf("GetBundle = %q, %v", got, err)
	}
	if sum, err := s.Stat(ctx, slug, ver); err != nil || sum != Hash(v1) {
		t.Fatalf("Stat = %q, %v; want %q", sum, err, Hash(v1))
	}

	// Same test-set version, different bytes (new limits in problem.yaml): replaced.
	v2 := []byte("bundle two")
	if wrote, err := s.PutBundle(ctx, slug, ver, v2); err != nil || !wrote {
		t.Fatalf("changed put: wrote=%v err=%v", wrote, err)
	}
	if got, _ := s.GetBundle(ctx, slug, ver); !bytes.Equal(got, v2) {
		t.Fatalf("after replace GetBundle = %q", got)
	}
}
