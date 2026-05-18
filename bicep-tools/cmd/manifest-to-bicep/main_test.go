package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGenerate_SingleFile(t *testing.T) {
	outputDir := t.TempDir()

	err := RunGenerate([]string{"testdata/containers.yaml"}, outputDir)
	if err != nil {
		t.Fatalf("RunGenerate returned error: %v", err)
	}

	// Verify all three output files are created.
	for _, filename := range []string{"types.json", "index.json", "index.md"} {
		path := filepath.Join(outputDir, filename)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected %s to exist: %v", filename, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("expected %s to be non-empty", filename)
		}
	}
}

func TestRunGenerate_MultipleFiles_SameNamespace(t *testing.T) {
	outputDir := t.TempDir()

	err := RunGenerate([]string{"testdata/containers.yaml", "testdata/routes.yaml"}, outputDir)
	if err != nil {
		t.Fatalf("RunGenerate returned error: %v", err)
	}

	// Verify all three output files are created.
	for _, filename := range []string{"types.json", "index.json", "index.md"} {
		path := filepath.Join(outputDir, filename)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected %s to exist: %v", filename, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("expected %s to be non-empty", filename)
		}
	}

	// Verify the merged index.json references both resource types.
	indexContent, err := os.ReadFile(filepath.Join(outputDir, "index.json"))
	if err != nil {
		t.Fatalf("failed to read index.json: %v", err)
	}

	indexStr := string(indexContent)
	for _, typeName := range []string{"Radius.Compute/containers", "Radius.Compute/routes"} {
		if !strings.Contains(indexStr, typeName) {
			t.Errorf("expected index.json to contain %q, but it was not found", typeName)
		}
	}
}

func TestRunGenerate_MultipleFiles_DifferentNamespaces(t *testing.T) {
	outputDir := t.TempDir()

	err := RunGenerate([]string{"testdata/containers.yaml", "testdata/secrets.yaml"}, outputDir)
	if err == nil {
		t.Fatal("expected error when merging manifests with different namespaces, got nil")
	}

	expected := "all manifests must share the same namespace"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error containing %q, got %q", expected, err.Error())
	}
}

func TestRunGenerate_NonexistentFile(t *testing.T) {
	outputDir := t.TempDir()

	err := RunGenerate([]string{"testdata/nonexistent.yaml"}, outputDir)
	if err == nil {
		t.Fatal("expected error for nonexistent manifest file, got nil")
	}

	expected := "manifest file does not exist"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error containing %q, got %q", expected, err.Error())
	}
}

func TestRunGenerate_EmptyManifestList(t *testing.T) {
	outputDir := t.TempDir()

	err := RunGenerate([]string{}, outputDir)
	if err == nil {
		t.Fatal("expected error for empty manifest list, got nil")
	}

	expected := "at least one manifest file is required"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error containing %q, got %q", expected, err.Error())
	}
}

func TestMergeManifestFiles_DuplicateType(t *testing.T) {
	// Both files define "containers" in Radius.Compute - should be rejected.
	_, err := mergeManifestFiles([]string{"testdata/containers.yaml", "testdata/containers.yaml"})
	if err == nil {
		t.Fatal("expected error for duplicate resource type, got nil")
	}

	expected := "duplicate resource type"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error containing %q, got %q", expected, err.Error())
	}
}
