// Package storage keeps problem bundles (problem.yaml and tests, packed by
// judge/problem.Pack) in an S3-compatible bucket, so runners fetch the exact
// test set a submission was accepted against instead of reading a directory.
// It talks the plain S3 API, so the same code runs against the local RustFS
// container and against real S3.
package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MaxObjectBytes caps what Get will read, matching judge/problem.MaxBundleBytes.
const MaxObjectBytes = 64 << 20

const shaMeta = "Sha256"

// ErrNotFound means the bundle is not in the bucket.
var ErrNotFound = errors.New("storage: bundle not found")

var (
	slugRe    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	versionRe = regexp.MustCompile(`^ts-[0-9a-f]{16}$`)
)

// Config locates the bucket.
type Config struct {
	Endpoint  string // host:port, no scheme
	AccessKey string
	SecretKey string
	Bucket    string
	UseTLS    bool
}

// ConfigFromEnv reads LEETFORCE_S3_ENDPOINT, _ACCESS_KEY, _SECRET_KEY, _BUCKET
// and _USE_TLS. ok is false when no endpoint is set, which means "no object
// storage; read problems from disk". A set endpoint with missing credentials
// is an error, not a silent fallback.
func ConfigFromEnv() (cfg Config, ok bool, err error) {
	cfg = Config{
		Endpoint:  os.Getenv("LEETFORCE_S3_ENDPOINT"),
		AccessKey: os.Getenv("LEETFORCE_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("LEETFORCE_S3_SECRET_KEY"),
		Bucket:    os.Getenv("LEETFORCE_S3_BUCKET"),
		UseTLS:    os.Getenv("LEETFORCE_S3_USE_TLS") == "true",
	}
	if cfg.Endpoint == "" {
		return Config{}, false, nil
	}
	if cfg.Bucket == "" {
		cfg.Bucket = "leetforce-problems"
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return Config{}, false, errors.New("LEETFORCE_S3_ENDPOINT is set but LEETFORCE_S3_ACCESS_KEY or LEETFORCE_S3_SECRET_KEY is missing")
	}
	return cfg, true, nil
}

// Store is a handle on the bucket.
type Store struct {
	c      *minio.Client
	bucket string
}

// Open builds a client. It does not contact the server.
func Open(cfg Config) (*Store, error) {
	c, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseTLS,
	})
	if err != nil {
		return nil, fmt.Errorf("open object storage: %w", err)
	}
	return &Store{c: c, bucket: cfg.Bucket}, nil
}

// Ping checks that the server answers and the bucket exists.
func (s *Store) Ping(ctx context.Context) error {
	ok, err := s.c.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check bucket: %w", err)
	}
	if !ok {
		return errors.New("check bucket: bucket does not exist")
	}
	return nil
}

// EnsureBucket creates the bucket if it is missing.
func (s *Store) EnsureBucket(ctx context.Context) error {
	ok, err := s.c.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check bucket: %w", err)
	}
	if ok {
		return nil
	}
	if err := s.c.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		// another instance may have created it between the two calls
		if again, e2 := s.c.BucketExists(ctx, s.bucket); e2 == nil && again {
			return nil
		}
		return fmt.Errorf("create bucket: %w", err)
	}
	return nil
}

func key(slug, version string) (string, error) {
	if !slugRe.MatchString(slug) || !versionRe.MatchString(version) {
		return "", fmt.Errorf("storage: invalid bundle name %q version %q", slug, version)
	}
	return "problems/" + slug + "/" + version + ".tar.gz", nil
}

// Hash returns the hex SHA-256 of a bundle. It names the runner's cache
// directory, so a changed bundle (for example new limits in problem.yaml under
// the same test-set version) is never served from a stale cache.
func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// PutBundle stores a bundle under slug and test-set version, tagged with its
// SHA-256. If the object already holds exactly these bytes it does nothing and
// returns false; otherwise it writes and returns true.
func (s *Store) PutBundle(ctx context.Context, slug, version string, data []byte) (bool, error) {
	k, err := key(slug, version)
	if err != nil {
		return false, err
	}
	sum := Hash(data)
	if cur, err := s.Stat(ctx, slug, version); err == nil && cur == sum {
		return false, nil
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return false, err
	}
	_, err = s.c.PutObject(ctx, s.bucket, k, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType:  "application/gzip",
		UserMetadata: map[string]string{shaMeta: sum},
	})
	if err != nil {
		return false, fmt.Errorf("put bundle %s: %w", k, err)
	}
	return true, nil
}

// Stat returns the SHA-256 recorded for a bundle ("" if the object has none),
// or ErrNotFound.
func (s *Store) Stat(ctx context.Context, slug, version string) (string, error) {
	k, err := key(slug, version)
	if err != nil {
		return "", err
	}
	info, err := s.c.StatObject(ctx, s.bucket, k, minio.StatObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("stat bundle %s: %w", k, err)
	}
	for name, v := range info.UserMetadata {
		if strings.EqualFold(name, shaMeta) {
			return v, nil
		}
	}
	return "", nil
}

// GetBundle downloads a bundle (at most MaxObjectBytes) or returns ErrNotFound.
func (s *Store) GetBundle(ctx context.Context, slug, version string) ([]byte, error) {
	k, err := key(slug, version)
	if err != nil {
		return nil, err
	}
	obj, err := s.c.GetObject(ctx, s.bucket, k, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get bundle %s: %w", k, err)
	}
	defer func() { _ = obj.Close() }()
	data, err := io.ReadAll(io.LimitReader(obj, MaxObjectBytes+1))
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get bundle %s: %w", k, err)
	}
	if len(data) > MaxObjectBytes {
		return nil, fmt.Errorf("get bundle %s: object larger than %d bytes", k, MaxObjectBytes)
	}
	return data, nil
}

func isNotFound(err error) bool {
	return minio.ToErrorResponse(err).Code == "NoSuchKey"
}
