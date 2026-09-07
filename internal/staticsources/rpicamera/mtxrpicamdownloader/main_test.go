package main

import (
	"path/filepath"
	"testing"
)

func TestSafeArchivePath(t *testing.T) {
	baseDir := filepath.Clean(string(filepath.Separator) + filepath.Join("tmp", "base"))

	for _, ca := range []struct {
		name   string
		path   string
		ok     bool
		result string
	}{
		{
			name:   "simple file",
			path:   "dir/file",
			ok:     true,
			result: filepath.Join(baseDir, "dir", "file"),
		},
		{
			name: "parent traversal",
			path: "../file",
		},
		{
			name: "absolute path",
			path: "/etc/passwd",
		},
		{
			name: "empty path",
			path: "",
		},
		{
			name: "dot path",
			path: ".",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			res, err := safeArchivePath(baseDir, ca.path)
			if ca.ok {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if res != ca.result {
					t.Fatalf("unexpected result: got %q, want %q", res, ca.result)
				}
			} else if err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}
