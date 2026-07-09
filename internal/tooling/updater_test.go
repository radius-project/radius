package tooling

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateManifestRefreshesVersionAndChecksum(t *testing.T) {
	const checksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/latest":
			fmt.Fprint(response, `{"tag_name":"v2.0.0"}`)
		case "/checksums/v2.0.0/tool-v2.0.0.tar.gz":
			fmt.Fprintf(response, "%s  tool-v2.0.0.tar.gz\n", checksum)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	manifest := Manifest{
		SchemaVersion:        1,
		TerraformVersionFile: ".terraform-version",
		Platforms:            []string{"linux_amd64"},
		Tools: []Tool{{
			Name:       "tool",
			MakePrefix: "TOOL",
			Version:    "v1.0.0",
			Source: Source{
				Type:       "github-release",
				Repository: "example/tool",
				LatestURL:  server.URL + "/latest",
			},
			DownloadTemplate: server.URL + "/download/{tag}/{asset}",
			Platforms: map[string]Platform{
				"linux_amd64": {
					Asset:    "tool-{version}.tar.gz",
					Checksum: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				},
			},
			ChecksumSource: ChecksumSource{
				Type:        "url-file",
				URLTemplate: server.URL + "/checksums/{version}/{asset}",
				Format:      "standard",
			},
		}},
	}

	client := NewClient("")
	client.HTTP = server.Client()
	changes, err := UpdateManifest(context.Background(), &manifest, client)
	if err != nil {
		t.Fatalf("UpdateManifest() error = %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("got %d changes, want version and checksum changes", len(changes))
	}
	if got := manifest.Tools[0].Version; got != "v2.0.0" {
		t.Fatalf("version = %q, want v2.0.0", got)
	}
	if got := manifest.Tools[0].Platforms["linux_amd64"].Checksum; got != checksum {
		t.Fatalf("checksum = %q, want %q", got, checksum)
	}
}

func TestNewerVersionDoesNotDowngrade(t *testing.T) {
	newer, err := newerVersion("v1.9.0", "v2.0.0")
	if err != nil {
		t.Fatalf("newerVersion() error = %v", err)
	}
	if newer {
		t.Fatal("newerVersion() reported a downgrade as an upgrade")
	}
}

func TestChecksumCachesSharedFile(t *testing.T) {
	const (
		amd64Sum = "1111111111111111111111111111111111111111111111111111111111111111"
		arm64Sum = "2222222222222222222222222222222222222222222222222222222222222222"
	)

	var fetches int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/checksums" {
			fetches++
			fmt.Fprintf(response, "%s  tool_linux_amd64\n%s  tool_linux_arm64\n", amd64Sum, arm64Sum)
			return
		}
		http.NotFound(response, request)
	}))
	defer server.Close()

	tool := Tool{
		Name:             "tool",
		MakePrefix:       "TOOL",
		Version:          "v1.0.0",
		DownloadTemplate: server.URL + "/download/{asset}",
		Source: Source{
			Type:       "github-release",
			Repository: "example/tool",
			LatestURL:  server.URL + "/latest",
		},
		Platforms: map[string]Platform{
			"linux_amd64": {Asset: "tool_linux_amd64", Checksum: amd64Sum},
			"linux_arm64": {Asset: "tool_linux_arm64", Checksum: arm64Sum},
		},
		ChecksumSource: ChecksumSource{
			Type:        "url-file",
			URLTemplate: server.URL + "/checksums",
			Format:      "standard",
		},
	}

	client := NewClient("")
	client.HTTP = server.Client()

	for _, platform := range []string{"linux_amd64", "linux_arm64"} {
		if _, err := client.Checksum(context.Background(), tool, platform, tool.Version); err != nil {
			t.Fatalf("Checksum(%s) error = %v", platform, err)
		}
	}
	if fetches != 1 {
		t.Fatalf("shared checksum file fetched %d times, want 1", fetches)
	}
}
