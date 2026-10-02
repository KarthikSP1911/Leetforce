package problem

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// MaxBundleBytes caps a problem bundle, packed and unpacked, so a corrupt or
// hostile object in storage cannot fill a runner's disk or memory.
const MaxBundleBytes = 64 << 20

// Pack returns the problem at dir as a gzip-compressed tar holding only what a
// runner needs: problem.yaml and tests/. Reference solutions and anything else
// in the directory are left out. The bytes are deterministic (sorted entries,
// zero timestamps, fixed modes), so the same content always packs to the same
// object.
func Pack(dir string) ([]byte, error) {
	files := []string{"problem.yaml"}
	entries, err := os.ReadDir(filepath.Join(dir, "tests"))
	if err != nil {
		return nil, fmt.Errorf("pack problem: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			return nil, fmt.Errorf("pack problem: unexpected directory tests/%s", e.Name())
		}
		files = append(files, path.Join("tests", e.Name()))
	}
	sort.Strings(files)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name))) //nolint:gosec // operator-chosen dir
		if err != nil {
			return nil, fmt.Errorf("pack problem: %w", err)
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return nil, fmt.Errorf("pack problem: %w", err)
		}
		if _, err := tw.Write(data); err != nil {
			return nil, fmt.Errorf("pack problem: %w", err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("pack problem: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("pack problem: %w", err)
	}
	if buf.Len() > MaxBundleBytes {
		return nil, fmt.Errorf("pack problem: bundle is %d bytes, limit %d", buf.Len(), MaxBundleBytes)
	}
	return buf.Bytes(), nil
}

// Unpack extracts a bundle made by Pack into dest (which must exist). It
// accepts only regular files named problem.yaml or tests/<name>, rejects any
// other path (so "../x" or absolute names cannot escape dest), and stops at
// MaxBundleBytes of unpacked data.
func Unpack(data []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("unpack problem: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(io.LimitReader(gz, MaxBundleBytes+1))
	if err := os.MkdirAll(filepath.Join(dest, "tests"), 0o750); err != nil {
		return fmt.Errorf("unpack problem: %w", err)
	}
	var total int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("unpack problem: %w", err)
		}
		if h.Typeflag != tar.TypeReg || !bundlePathOK(h.Name) {
			return fmt.Errorf("unpack problem: unexpected entry %q", h.Name)
		}
		total += h.Size
		if total > MaxBundleBytes {
			return fmt.Errorf("unpack problem: more than %d bytes", MaxBundleBytes)
		}
		body, err := io.ReadAll(io.LimitReader(tr, h.Size+1))
		if err != nil || int64(len(body)) != h.Size {
			return fmt.Errorf("unpack problem: entry %q is truncated", h.Name)
		}
		if err := os.WriteFile(filepath.Join(dest, filepath.FromSlash(h.Name)), body, 0o600); err != nil {
			return fmt.Errorf("unpack problem: %w", err)
		}
	}
}

func bundlePathOK(name string) bool {
	if name == "problem.yaml" {
		return true
	}
	rest, ok := strings.CutPrefix(name, "tests/")
	return ok && rest != "" && !strings.ContainsAny(rest, `/\`) && rest != "." && rest != ".."
}
