package tooling

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallBicepCrossPlatform(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Bicep shell installer supports Linux and macOS")
	}
	t.Parallel()

	const pinned = "#!/bin/sh\n# pinned Bicep fixture; must not execute when cross-staging\nexit 99\n"
	const outdated = "#!/bin/sh\n# outdated Bicep fixture\nexit 99\n"
	// Read the script in Go so installer changes invalidate the test cache.
	script, err := os.ReadFile(filepath.Join("..", "..", "build", "scripts", "install-bicep.sh"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name         string
		existing     string
		download     string
		failDownload bool
		noChecksum   bool
		skipDownload bool
		wantError    bool
		want         string
	}{
		{name: "fresh install", download: pinned, want: pinned},
		{name: "replace outdated staged binary", existing: outdated, download: pinned, want: pinned},
		{name: "reuse checksum-verified binary", existing: pinned, skipDownload: true, want: pinned},
		{name: "redownload unverifiable staged binary", existing: outdated, download: pinned, noChecksum: true, want: pinned},
		{name: "reject invalid download without replacing binary", existing: outdated, download: "corrupted", wantError: true, want: outdated},
		{name: "preserve binary when download fails", existing: outdated, failDownload: true, wantError: true, want: outdated},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			installDir := filepath.Join(root, "install")
			for _, dir := range []string{bin, installDir} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			fixtures := map[string]string{
				filepath.Join(root, "install-bicep.sh"): string(script),
				filepath.Join(bin, "uname"): `#!/bin/sh
case "$1" in
    -s) echo Darwin ;;
    -m) echo x86_64 ;;
    *) exit 1 ;;
esac
`,
				filepath.Join(bin, "curl"): `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$CURL_LOG"
if [ "$FAIL_DOWNLOAD" = "true" ]; then
    echo "simulated download failure" >&2
    exit 22
fi
while [ "$#" -gt 0 ]; do
    if [ "$1" = "-o" ]; then
        cp "$DOWNLOAD_SOURCE" "$2"
        exit 0
    fi
    shift
done
exit 1
`,
				filepath.Join(root, "download"): test.download,
			}
			if test.existing != "" {
				fixtures[filepath.Join(installDir, "bicep")] = test.existing
			}
			for path, contents := range fixtures {
				if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(pinned)))
			if test.noChecksum {
				checksum = ""
			}
			command := exec.Command("bash", filepath.Join(root, "install-bicep.sh"), installDir)
			command.Env = append(os.Environ(),
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"BICEP_VERSION=v0.46.1",
				"BICEP_OS=linux",
				"BICEP_ARCH=arm64",
				"BICEP_CHECKSUM_LINUX_ARM64="+checksum,
				"GITHUB_TOKEN=",
				"GITHUB_PATH="+filepath.Join(root, "github-path"),
				"CURL_LOG="+filepath.Join(root, "curl.log"),
				"DOWNLOAD_SOURCE="+filepath.Join(root, "download"),
				fmt.Sprintf("FAIL_DOWNLOAD=%t", test.failDownload),
			)
			output, err := command.CombinedOutput()
			if (err != nil) != test.wantError {
				t.Fatalf("installer error = %v, wantError %v:\n%s", err, test.wantError, output)
			}
			contents, err := os.ReadFile(filepath.Join(installDir, "bicep"))
			if err != nil {
				t.Fatal(err)
			}
			if string(contents) != test.want {
				t.Fatalf("installed content = %q, want %q", contents, test.want)
			}
			requests, err := os.ReadFile(filepath.Join(root, "curl.log"))
			if test.skipDownload {
				if !os.IsNotExist(err) {
					t.Fatalf("verified binary should not be downloaded again: %s, %v", requests, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(requests), "https://github.com/Azure/bicep/releases/download/v0.46.1/bicep-linux-arm64") {
					t.Fatalf("unexpected download request: %s", requests)
				}
			}
			if _, err := os.Stat(filepath.Join(root, "github-path")); !os.IsNotExist(err) {
				t.Fatalf("cross-platform staging must not export PATH: %v", err)
			}
		})
	}
}
