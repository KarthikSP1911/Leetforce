package problem

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestPackUnpackRoundTrip(t *testing.T) {
	src := writeProblem(t, validYAML, okFiles)
	if err := os.MkdirAll(filepath.Join(src, "solutions", "python"), 0o750); err != nil {
		t.Fatal(err)
	}
	// A reference solution must never travel in the bundle.
	if err := os.WriteFile(filepath.Join(src, "solutions", "python", "ac.py"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	orig, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}

	data, err := Pack(src)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Pack(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, again) {
		t.Error("Pack is not deterministic")
	}

	dst := t.TempDir()
	if err := Unpack(data, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "solutions")); err == nil {
		t.Error("solutions/ was bundled")
	}
	got, err := Load(dst)
	if err != nil {
		t.Fatal(err)
	}
	if got.TestSetVer != orig.TestSetVer {
		t.Errorf("version after round trip = %s, want %s", got.TestSetVer, orig.TestSetVer)
	}
}

func TestUnpackRejectsBadEntries(t *testing.T) {
	tests := []struct {
		name string
		hdr  tar.Header
		body string
	}{
		{"parent path", tar.Header{Name: "../evil", Typeflag: tar.TypeReg}, "x"},
		{"absolute path", tar.Header{Name: "/etc/evil", Typeflag: tar.TypeReg}, "x"},
		{"nested tests dir", tar.Header{Name: "tests/a/b.in", Typeflag: tar.TypeReg}, "x"},
		{"unknown top-level file", tar.Header{Name: "run.sh", Typeflag: tar.TypeReg}, "x"},
		{"symlink", tar.Header{Name: "tests/a.in", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}, ""},
		{"directory", tar.Header{Name: "tests/", Typeflag: tar.TypeDir}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gz)
			h := tc.hdr
			h.Size = int64(len(tc.body))
			h.Mode = 0o644
			if err := tw.WriteHeader(&h); err != nil {
				t.Fatal(err)
			}
			_, _ = tw.Write([]byte(tc.body))
			_ = tw.Close()
			_ = gz.Close()
			dst := t.TempDir()
			if err := Unpack(buf.Bytes(), dst); err == nil {
				t.Fatal("Unpack accepted a bad entry")
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(dst), "evil")); err == nil {
				t.Fatal("file escaped the destination")
			}
		})
	}
}

func TestUnpackRejectsGarbage(t *testing.T) {
	if err := Unpack([]byte("not a gzip"), t.TempDir()); err == nil {
		t.Fatal("Unpack accepted garbage")
	}
}
