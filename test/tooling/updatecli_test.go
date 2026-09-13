package tooling_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type manifest struct {
	SchemaVersion int      `yaml:"schemaVersion"`
	Platforms     []string `yaml:"platforms"`
	Tools         []tool   `yaml:"tools"`
}

type tool struct {
	Name           string              `yaml:"name"`
	MakePrefix     string              `yaml:"makePrefix"`
	Version        string              `yaml:"version"`
	Update         *bool               `yaml:"update,omitempty"`
	Source         source              `yaml:"source"`
	Platforms      map[string]platform `yaml:"platforms,omitempty"`
	ChecksumSource struct {
		Type        string `yaml:"type"`
		URLTemplate string `yaml:"urlTemplate,omitempty"`
		Format      string `yaml:"format,omitempty"`
		Integrity   string `yaml:"integrity,omitempty"`
	} `yaml:"checksumSource"`
	VersionFiles []versionFile `yaml:"versionFiles,omitempty"`
}

type source struct {
	Type       string `yaml:"type"`
	Repository string `yaml:"repository,omitempty"`
	TagPrefix  string `yaml:"tagPrefix,omitempty"`
	LatestURL  string `yaml:"latestURL"`
}

type platform struct {
	Asset    string `yaml:"asset"`
	Checksum string `yaml:"checksum"`
	OS       string `yaml:"os,omitempty"`
	Arch     string `yaml:"arch,omitempty"`
}

type versionFile struct {
	Path   string `yaml:"path"`
	Format string `yaml:"format"`
	Prefix string `yaml:"prefix,omitempty"`
	Suffix string `yaml:"suffix,omitempty"`
	Key    string `yaml:"key,omitempty"`
}

type fixture struct {
	root       string
	binary     string
	manifest   manifest
	candidates map[string]string
	server     *httptest.Server
	mode       string
}

func TestUpdatecliVersionPin(t *testing.T) {
	t.Parallel()
	pin := strings.TrimSpace(string(readFile(t, filepath.Join("..", "..", ".updatecli-version"))))
	if !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(pin) {
		t.Fatalf("Updatecli's setup action requires a v-prefixed release tag, got %q", pin)
	}
}

func TestUpdateToolsApply(t *testing.T) {
	for _, mode := range []string{"upgrade", "same-version", "older-release"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, mode)
			if output, err := f.run(t, "apply"); err != nil {
				t.Fatalf("Updatecli apply failed: %v\n%s", err, output)
			}
			f.assertOutputs(t)
			first := f.snapshot(t)
			if output, err := f.run(t, "apply"); err != nil {
				t.Fatalf("second Updatecli apply failed: %v\n%s", err, output)
			}
			assertSnapshot(t, first, f.snapshot(t))
		})
	}
}

func TestUpdateToolsDiffDoesNotWritePins(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "upgrade")
	before := f.snapshot(t)
	if output, err := f.run(t, "diff"); err != nil {
		t.Fatalf("Updatecli diff failed: %v\n%s", err, output)
	}
	assertSnapshot(t, before, f.snapshot(t))
}

func TestUpdateToolsFailureDoesNotWritePins(t *testing.T) {
	for _, mode := range []string{"missing-asset", "duplicate-asset", "invalid-digest", "invalid-version", "missing-consumer"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, mode)
			if mode == "missing-consumer" {
				if err := os.Remove(filepath.Join(f.root, ".terraform-version")); err != nil {
					t.Fatal(err)
				}
			}
			before := f.snapshot(t)
			if output, err := f.run(t, "apply"); err == nil {
				t.Fatalf("Updatecli unexpectedly accepted %s:\n%s", mode, output)
			}
			assertSnapshot(t, before, f.snapshot(t))
		})
	}
}

func TestGenerateToolsIsOffline(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "same-version")
	f.server.Close()
	if output, err := f.run(t, "apply", "--values-inline", "refresh: false"); err != nil {
		t.Fatalf("offline generation failed: %v\n%s", err, output)
	}
	got := readManifest(t, filepath.Join(f.root, "build", "tools.yaml"))
	for index, entry := range got.Tools {
		if entry.Version != f.manifest.Tools[index].Version {
			t.Errorf("offline generation changed %s version", entry.Name)
		}
		for target, asset := range entry.Platforms {
			if asset.Checksum != f.manifest.Tools[index].Platforms[target].Checksum {
				t.Errorf("offline generation changed %s %s checksum", entry.Name, target)
			}
		}
	}
	if !strings.Contains(string(readFile(t, filepath.Join(f.root, "build", "tools.generated.mk"))), "YQ_VERSION ?=") {
		t.Fatal("offline generation did not render Make metadata")
	}
}

func TestUpdateToolsTargetFailureIsError(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "upgrade")
	if err := os.Mkdir(filepath.Join(f.root, "blocked-output"), 0o755); err != nil {
		t.Fatal(err)
	}
	if output, err := f.run(t, "apply", "--values-inline", "makefilePath: blocked-output"); err == nil {
		t.Fatalf("Updatecli accepted an unwritable output target:\n%s", output)
	}
}

func TestCommittedMakeMetadata(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "same-version")
	if output, err := f.run(t, "apply", "--values-inline", "refresh: false"); err != nil {
		t.Fatalf("offline generation failed: %v\n%s", err, output)
	}
	generated := strings.ReplaceAll(string(readFile(t, filepath.Join(f.root, "build", "tools.generated.mk"))), "\r\n", "\n")
	committed := strings.ReplaceAll(string(readFile(t, filepath.Join("..", "..", "build", "tools.generated.mk"))), "\r\n", "\n")
	if generated != committed {
		t.Fatal("committed Make metadata is stale; run make generate-tools")
	}
}

func newFixture(t *testing.T, mode string) *fixture {
	t.Helper()
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		root:       t.TempDir(),
		binary:     updatecliBinary(t, repository),
		manifest:   readManifest(t, filepath.Join(repository, "build", "tools.yaml")),
		candidates: make(map[string]string),
		mode:       mode,
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.server.Close)

	for index := range f.manifest.Tools {
		entry := &f.manifest.Tools[index]
		candidate := "99.0.0"
		if mode == "older-release" {
			candidate = "0.0.1"
		}
		if strings.HasPrefix(entry.Version, "v") {
			candidate = "v" + candidate
		}
		if mode == "same-version" {
			candidate = entry.Version
		}
		if mode == "invalid-version" && entry.Name == "yq" {
			candidate = "not-a-version"
		}
		f.candidates[entry.Name] = candidate
		entry.Source.LatestURL = f.server.URL + "/latest/" + entry.Name
		if entry.ChecksumSource.Type == "url-file" {
			entry.ChecksumSource.URLTemplate = f.server.URL + "/checksums/" + entry.Name + "/{version}/{os}/{arch}"
		}
		for _, consumer := range entry.VersionFiles {
			content := "0.0.1\n"
			if consumer.Format == "replace" {
				content = consumer.Prefix + "0.0.1" + consumer.Suffix + "\n"
			} else if consumer.Format == "yaml" {
				content = "global:\n  terraform:\n    version: \"0.0.1\"\n"
			}
			writeFile(t, filepath.Join(f.root, filepath.FromSlash(consumer.Path)), []byte(content))
		}
	}
	contents, err := yaml.Marshal(f.manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(f.root, "build", "tools.yaml"), contents)
	writeFile(t, filepath.Join(f.root, "build", "tools.generated.mk"), []byte("# original Make metadata\n"))
	for _, name := range []string{"update-tools.yaml.tpl", "tools.mk.tpl", "version.tpl"} {
		writeFile(t, filepath.Join(f.root, ".updatecli", name), readFile(t, filepath.Join(repository, ".updatecli", name)))
	}
	return f
}

func updatecliBinary(t *testing.T, repository string) string {
	t.Helper()
	binary := os.Getenv("UPDATECLI")
	if binary == "" && runtime.GOOS == "windows" {
		version := strings.TrimSpace(string(readFile(t, filepath.Join(repository, ".updatecli-version"))))
		binary = filepath.Join(repository, "bin", "updatecli-"+version, "updatecli.exe")
	}
	if binary == "" {
		binary = "updatecli"
	}
	if !filepath.IsAbs(binary) && strings.ContainsAny(binary, `/\`) {
		binary = filepath.Join(repository, binary)
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		t.Fatalf("install the pinned Updatecli CLI or set UPDATECLI to its executable: %v", err)
	}
	return resolved
}

func (f *fixture) run(t *testing.T, action string, additional ...string) ([]byte, error) {
	t.Helper()
	args := []string{
		"--disable-version-check", "--unique-tmp-dir", "pipeline", action,
		"--config", ".updatecli/update-tools.yaml.tpl", "--values", "build/tools.yaml",
		"--values-inline", "githubAPI: " + f.server.URL,
		"--disable-changelog", "--disable-udash-report", "--validate-schema",
	}
	args = append(args, additional...)
	command := exec.CommandContext(t.Context(), f.binary, args...)
	command.Dir = f.root
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		switch strings.ToUpper(name) {
		case "GITHUB_TOKEN", "GH_TOKEN", "UPDATECLI_GITHUB_TOKEN":
			continue
		}
		command.Env = append(command.Env, variable)
	}
	return command.CombinedOutput()
}

func (f *fixture) expectedVersion(entry tool) string {
	if f.mode == "older-release" || (entry.Update != nil && !*entry.Update) {
		return entry.Version
	}
	return f.candidates[entry.Name]
}

func fixtureChecksum(entry tool, target, version string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(entry.Name+"/"+target+"/"+version)))
}

func assetName(entry tool, asset platform, target, version string) string {
	osName, arch, _ := strings.Cut(target, "_")
	if asset.OS != "" {
		osName = asset.OS
	}
	if asset.Arch != "" {
		arch = asset.Arch
	}
	return strings.NewReplacer(
		"{version_no_v}", strings.TrimPrefix(version, "v"),
		"{version}", version,
		"{tag}", entry.Source.TagPrefix+version,
		"{repository}", entry.Source.Repository,
		"{os}", osName,
		"{arch}", arch,
	).Replace(asset.Asset)
}

func (f *fixture) serveHTTP(response http.ResponseWriter, request *http.Request) {
	for _, entry := range f.manifest.Tools {
		if request.URL.Path == "/latest/"+entry.Name {
			response.Header().Set("Content-Type", "application/json")
			version := f.candidates[entry.Name]
			switch entry.Source.Type {
			case "github-release":
				fmt.Fprintf(response, `{"tag_name":%q,"published_at":"2099-01-01T00:00:00Z"}`, entry.Source.TagPrefix+version)
			case "hashicorp-checkpoint":
				fmt.Fprintf(response, `{"current_version":%q}`, version)
			case "stable-text":
				fmt.Fprintln(response, version)
			}
			return
		}
		version := f.expectedVersion(entry)
		if request.URL.Path == "/repos/"+entry.Source.Repository+"/releases/tags/"+entry.Source.TagPrefix+version {
			type releaseAsset struct {
				Name   string `json:"name"`
				Digest string `json:"digest"`
			}
			assets := []releaseAsset{}
			for target, asset := range entry.Platforms {
				if entry.Name == "yq" && target == "linux_arm64" && f.mode == "missing-asset" {
					continue
				}
				record := releaseAsset{
					Name:   assetName(entry, asset, target, version),
					Digest: "sha256:" + fixtureChecksum(entry, target, version),
				}
				if entry.Name == "yq" && target == "linux_arm64" && f.mode == "invalid-digest" {
					record.Digest = "sha256:not-a-checksum"
				}
				assets = append(assets, record)
				if entry.Name == "yq" && target == "linux_arm64" && f.mode == "duplicate-asset" {
					assets = append(assets, record)
				}
			}
			response.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(response).Encode(struct {
				Assets []releaseAsset `json:"assets"`
			}{assets}); err != nil {
				http.Error(response, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		for target, asset := range entry.Platforms {
			osName, arch, _ := strings.Cut(target, "_")
			if request.URL.Path == "/checksums/"+entry.Name+"/"+version+"/"+osName+"/"+arch {
				fmt.Fprintf(response, "%s  %s\n", fixtureChecksum(entry, target, version), assetName(entry, asset, target, version))
				return
			}
		}
	}
	http.NotFound(response, request)
}

func (f *fixture) assertOutputs(t *testing.T) {
	t.Helper()
	got := readManifest(t, filepath.Join(f.root, "build", "tools.yaml"))
	makefile := strings.ReplaceAll(string(readFile(t, filepath.Join(f.root, "build", "tools.generated.mk"))), "\r\n", "\n")
	checksums := 0
	if len(got.Tools) != len(f.manifest.Tools) {
		t.Fatalf("got %d tools, want the complete %d-tool inventory", len(got.Tools), len(f.manifest.Tools))
	}
	for index, entry := range got.Tools {
		version := f.expectedVersion(f.manifest.Tools[index])
		if entry.Version != version {
			t.Errorf("%s version = %q, want %q", entry.Name, entry.Version, version)
		}
		if !strings.Contains(makefile, entry.MakePrefix+"_VERSION ?= "+version+"\n") {
			t.Errorf("Make metadata does not contain %s version %s", entry.Name, version)
		}
		for target, asset := range entry.Platforms {
			checksums++
			want := fixtureChecksum(entry, target, version)
			if asset.Checksum != want {
				t.Errorf("%s %s checksum = %q, want %q", entry.Name, target, asset.Checksum, want)
			}
			if !strings.Contains(makefile, entry.MakePrefix+"_CHECKSUM_"+strings.ToUpper(target)+" ?= "+want+"\n") {
				t.Errorf("Make metadata does not contain %s %s checksum", entry.Name, target)
			}
		}
		for _, consumer := range entry.VersionFiles {
			contents := string(readFile(t, filepath.Join(f.root, filepath.FromSlash(consumer.Path))))
			want := version + "\n"
			if consumer.Format == "replace" {
				want = consumer.Prefix + version + consumer.Suffix + "\n"
			} else if consumer.Format == "yaml" {
				var chart struct {
					Global struct {
						Terraform struct {
							Version string `yaml:"version"`
						} `yaml:"terraform"`
					} `yaml:"global"`
				}
				if err := yaml.Unmarshal([]byte(contents), &chart); err != nil {
					t.Fatal(err)
				}
				if chart.Global.Terraform.Version != version {
					t.Errorf("%s has version %q, want %q", consumer.Path, chart.Global.Terraform.Version, version)
				}
				continue
			}
			if contents != want {
				t.Errorf("%s = %q, want %q", consumer.Path, contents, want)
			}
		}
	}
	expectedChecksums := 0
	for _, entry := range f.manifest.Tools {
		expectedChecksums += len(entry.Platforms)
	}
	if checksums != expectedChecksums {
		t.Errorf("got %d platform checksums, want %d", checksums, expectedChecksums)
	}
}

func (f *fixture) snapshot(t *testing.T) map[string][]byte {
	t.Helper()
	files := []string{"build/tools.yaml", "build/tools.generated.mk"}
	for _, entry := range f.manifest.Tools {
		for _, consumer := range entry.VersionFiles {
			files = append(files, consumer.Path)
		}
	}
	result := make(map[string][]byte, len(files))
	for _, name := range files {
		contents, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(name)))
		if os.IsNotExist(err) {
			result[name] = nil
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		result[name] = contents
	}
	return result
}

func assertSnapshot(t *testing.T, want, got map[string][]byte) {
	t.Helper()
	for name, contents := range want {
		if !bytes.Equal(contents, got[name]) {
			t.Errorf("%s changed unexpectedly", name)
		}
	}
}

func readManifest(t *testing.T, path string) manifest {
	t.Helper()
	var result manifest
	if err := yaml.Unmarshal(readFile(t, path), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func writeFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}
