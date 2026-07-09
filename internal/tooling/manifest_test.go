package tooling

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRepositoryManifest(t *testing.T) {
	buildPath := filepath.Join("..", "..", "build")
	manifestPath := filepath.Join(buildPath, "tools.yaml")
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	contents, err := GenerateMake(manifest)
	if err != nil {
		t.Fatalf("GenerateMake() error = %v", err)
	}
	committed, err := os.ReadFile(filepath.Join(buildPath, "tools.generated.mk"))
	if err != nil {
		t.Fatalf("read committed Make metadata: %v", err)
	}
	if string(contents) != string(committed) {
		t.Fatal("generated Make metadata differs from build/tools.generated.mk; run make update-tools")
	}
}

func TestLoadManifestRejectsUnknownFields(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "build", "tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	invalid := strings.Replace(string(contents), "update: false", "updat: false", 1)
	if invalid == string(contents) {
		t.Fatal("test fixture does not contain update: false")
	}
	path := filepath.Join(t.TempDir(), "tools.yaml")
	if err := os.WriteFile(path, []byte(invalid), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = LoadManifest(path)
	if err == nil || !strings.Contains(err.Error(), "field updat not found") {
		t.Fatalf("LoadManifest() error = %v, want unknown-field error", err)
	}
}

func TestManifestValidationRejectsInvalidSourcesAndFormats(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Manifest)
		want  string
	}{
		{
			name: "unsupported source type",
			setup: func(manifest *Manifest) {
				manifest.Tools[0].Source.Type = "github"
			},
			want: "unsupported version source",
		},
		{
			name: "insecure latest URL",
			setup: func(manifest *Manifest) {
				manifest.Tools[0].Source.LatestURL = "http://example.test/latest"
			},
			want: "valid HTTPS URL",
		},
		{
			name: "insecure download URL",
			setup: func(manifest *Manifest) {
				manifest.Tools[0].DownloadTemplate = "http://example.test/{version}/{asset}"
			},
			want: "downloadTemplate must expand to a valid HTTPS URL",
		},
		{
			name: "insecure checksum URL",
			setup: func(manifest *Manifest) {
				manifest.Tools[0].ChecksumSource.URLTemplate = "http://example.test/checksums"
			},
			want: "checksumSource.urlTemplate must expand to a valid HTTPS URL",
		},
		{
			name: "unsupported checksum format",
			setup: func(manifest *Manifest) {
				manifest.Tools[0].ChecksumSource.Format = "md5"
			},
			want: "unsupported checksum format",
		},
		{
			name: "yq format for URL checksum source",
			setup: func(manifest *Manifest) {
				manifest.Tools[0].ChecksumSource.Format = "yq"
			},
			want: "unsupported checksum format",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validManifestForValidation()
			test.setup(&manifest)
			err := manifest.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func validManifestForValidation() Manifest {
	return Manifest{
		SchemaVersion: 1,
		Platforms:     []string{"linux_amd64"},
		Tools: []Tool{{
			Name:       "tool",
			MakePrefix: "TOOL",
			Version:    "v1.0.0",
			Source: Source{
				Type:      "stable-text",
				LatestURL: "https://example.test/latest",
			},
			DownloadTemplate: "https://example.test/{version}/{asset}",
			Platforms: map[string]Platform{
				"linux_amd64": {
					Asset:    "tool",
					Checksum: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				},
			},
			ChecksumSource: ChecksumSource{
				Type:        "url-file",
				URLTemplate: "https://example.test/checksums",
				Format:      "standard",
			},
		}},
	}
}

func TestExpandTemplate(t *testing.T) {
	values := map[string]string{
		"version": "v4.2.3",
		"asset":   "helm-v4.2.3-linux-amd64.tar.gz",
	}
	got, err := ExpandTemplate("https://example.test/{version}/{asset}", values)
	if err != nil {
		t.Fatalf("ExpandTemplate() error = %v", err)
	}
	want := "https://example.test/v4.2.3/helm-v4.2.3-linux-amd64.tar.gz"
	if got != want {
		t.Fatalf("ExpandTemplate() = %q, want %q", got, want)
	}

	if _, err := ExpandTemplate("{missing}", values); err == nil {
		t.Fatal("ExpandTemplate() accepted an unknown variable")
	}
}

func TestParseChecksum(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		content string
		asset   string
		want    string
	}{
		{
			name:    "standard",
			format:  "standard",
			content: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  tool.tar.gz\n",
			asset:   "tool.tar.gz",
			want:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		{
			name:    "basename",
			format:  "basename",
			content: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  _dist/tool\n",
			asset:   "tool",
			want:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		{
			name:    "first",
			format:  "first",
			content: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  tool\n",
			asset:   "tool",
			want:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseChecksum([]byte(test.content), test.format, test.asset)
			if err != nil {
				t.Fatalf("parseChecksum() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("parseChecksum() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseYQChecksum(t *testing.T) {
	checksum := "asset 1111111111111111111111111111111111111111111111111111111111111111 2222222222222222222222222222222222222222222222222222222222222222\n"
	got, err := parseYQChecksum([]byte("MD5\nSHA-256\n"), []byte(checksum), "asset")
	if err != nil {
		t.Fatalf("parseYQChecksum() error = %v", err)
	}
	want := "2222222222222222222222222222222222222222222222222222222222222222"
	if got != want {
		t.Fatalf("parseYQChecksum() = %q, want %q", got, want)
	}
}

func TestSyncVersionFiles(t *testing.T) {
	root := t.TempDir()
	goFile := filepath.Join(root, "version.go")
	chartFile := filepath.Join(root, "values.yaml")
	if err := os.WriteFile(goFile, []byte("var terraformVersion = \"1.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chartFile, []byte("    version: \"1.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := Tool{
		Name:    "terraform",
		Version: "1.1.0",
		VersionFiles: []VersionFile{
			{
				Path:   "version.go",
				Format: "replace",
				Prefix: `var terraformVersion = "`,
				Suffix: `"`,
			},
			{
				Path:   "values.yaml",
				Format: "replace",
				Prefix: `    version: "`,
				Suffix: `"`,
			},
		},
	}
	if err := SyncVersionFiles(root, tool); err != nil {
		t.Fatalf("SyncVersionFiles() error = %v", err)
	}

	for _, path := range []string{goFile, chartFile} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(contents), "1.1.0") {
			t.Errorf("%s does not contain the updated version: %s", path, contents)
		}
	}
}

func TestWriteManifestPreservesComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools.yaml")
	contents := `schemaVersion: 1
platforms:
  - linux_amd64
tools:
  - name: tool
    makePrefix: TOOL
    version: v1.0.0 # keep this inline note
    source:
      type: stable-text
      latestURL: https://example.test/stable
    downloadTemplate: https://example.test/{version}
    platforms:
      linux_amd64:
        asset: tool
        checksum: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    checksumSource:
      type: download
    # Keep the reason for this tool close to its metadata.
    notes: important tool
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	manifest.Tools[0].Version = "v1.1.0"
	manifest.Tools[0].Platforms["linux_amd64"] = Platform{
		Asset:    "tool",
		Checksum: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if _, err := WriteManifest(path, manifest); err != nil {
		t.Fatalf("WriteManifest() error = %v", err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	for _, expected := range []string{
		"version: v1.1.0 # keep this inline note",
		"checksum: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"# Keep the reason for this tool close to its metadata.",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("updated manifest does not contain %q:\n%s", expected, text)
		}
	}
}
