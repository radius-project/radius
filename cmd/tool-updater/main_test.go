package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/internal/tooling"
)

func TestSyncVersionFilesUpdatesEveryTool(t *testing.T) {
	root := t.TempDir()
	manifest := tooling.Manifest{
		Tools: []tooling.Tool{
			{
				Name:    "first",
				Version: "1.2.3",
				VersionFiles: []tooling.VersionFile{{
					Path:   "first.version",
					Format: "plain",
				}},
			},
			{
				Name:    "second",
				Version: "v4.5.6",
				VersionFiles: []tooling.VersionFile{{
					Path:   "second.version",
					Format: "plain",
				}},
			},
		},
	}
	for _, name := range []string{"first.version", "second.version"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := syncVersionFiles(root, manifest); err != nil {
		t.Fatalf("syncVersionFiles() error = %v", err)
	}
	for path, want := range map[string]string{
		"first.version":  "1.2.3\n",
		"second.version": "v4.5.6\n",
	} {
		contents, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		if got := string(contents); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}
