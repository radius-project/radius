package tooling

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"time"
)

type httpClientFunc func(*http.Request) (*http.Response, error)

func (function httpClientFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

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

	manifest := githubToolManifest(server.URL, 0)
	client := NewClient("")
	client.HTTP = server.Client()
	result, err := UpdateManifest(t.Context(), &manifest, client)
	if err != nil {
		t.Fatalf("UpdateManifest() error = %v", err)
	}
	if len(result.Changes) != 2 {
		t.Fatalf("got %d changes, want version and checksum changes", len(result.Changes))
	}
	if len(result.VersionUpdates) != 1 {
		t.Fatalf("got %d version updates, want 1", len(result.VersionUpdates))
	}
	versionUpdate := result.VersionUpdates[0]
	if versionUpdate.ReleaseURL != "https://github.com/example/tool/releases/tag/v2.0.0" {
		t.Errorf("release URL = %q, want GitHub release URL", versionUpdate.ReleaseURL)
	}
	if got := manifest.Tools[0].Version; got != "v2.0.0" {
		t.Fatalf("version = %q, want v2.0.0", got)
	}
	if got := manifest.Tools[0].Platforms["linux_amd64"].Checksum; got != checksum {
		t.Fatalf("checksum = %q, want %q", got, checksum)
	}
}

func TestUpdateResultPullRequestBodyMarkdown(t *testing.T) {
	result := UpdateResult{VersionUpdates: []VersionUpdate{
		{
			Tool:            "linked-tool",
			PreviousVersion: "v1.0.0",
			NewVersion:      "v2.0.0",
			ReleaseURL:      "https://github.com/example/tool/releases/tag/v2.0.0",
		},
		{
			Tool:            "unlinked-tool",
			PreviousVersion: "1.0.0",
			NewVersion:      "2.0.0",
		},
	}}

	want := "This automated PR refreshes the pinned command-line tool versions and SHA-256\n" +
		"checksums from the release sources declared in `build/tools.yaml`.\n\n" +
		"`build/tools.generated.mk` is generated from the manifest and committed with\n" +
		"it. Bicep remains intentionally held at its compatibility-pinned version until\n" +
		"local `br:localhost` functional tests support a newer release.\n\n" +
		"## Updated tool releases\n\n" +
		"- `linked-tool`: `v1.0.0` -> [`v2.0.0` release notes](https://github.com/example/tool/releases/tag/v2.0.0)\n" +
		"- `unlinked-tool`: `1.0.0` -> `2.0.0`\n"
	if got := result.PullRequestBodyMarkdown(); got != want {
		t.Fatalf("PullRequestBodyMarkdown() = %q, want %q", got, want)
	}

	withoutUpdates := (UpdateResult{}).PullRequestBodyMarkdown()
	if !strings.Contains(withoutUpdates, "build/tools.yaml") {
		t.Fatalf("body without version updates is incomplete: %q", withoutUpdates)
	}
}

func TestUpdateManifestAppliesReleaseCooldown(t *testing.T) {
	const checksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	tests := []struct {
		name        string
		publishedAt time.Time
		wantVersion string
		wantHeld    int
	}{
		{
			name:        "release inside the cooldown stays pinned",
			publishedAt: time.Now().Add(-2 * 24 * time.Hour),
			wantVersion: "v1.0.0",
			wantHeld:    1,
		},
		{
			name:        "release past the cooldown is adopted",
			publishedAt: time.Now().Add(-30 * 24 * time.Hour),
			wantVersion: "v2.0.0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/latest" {
					fmt.Fprintf(response, `{"tag_name":"v2.0.0","published_at":%q}`, test.publishedAt.UTC().Format(time.RFC3339))
					return
				}
				if strings.HasPrefix(request.URL.Path, "/checksums/") {
					fmt.Fprintf(response, "%s  %s\n", checksum, path.Base(request.URL.Path))
					return
				}
				http.NotFound(response, request)
			}))
			defer server.Close()

			manifest := githubToolManifest(server.URL, 7)
			client := NewClient("")
			client.HTTP = server.Client()
			result, err := UpdateManifest(t.Context(), &manifest, client)
			if err != nil {
				t.Fatalf("UpdateManifest() error = %v", err)
			}
			if got := manifest.Tools[0].Version; got != test.wantVersion {
				t.Errorf("version = %q, want %q", got, test.wantVersion)
			}
			if got := len(result.Held); got != test.wantHeld {
				t.Errorf("held = %d (%v), want %d", got, result.Held, test.wantHeld)
			}
		})
	}
}

func TestUpdateManifestRejectsUnknownReleaseDateDuringCooldown(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/latest" {
			fmt.Fprint(response, `{"tag_name":"v2.0.0"}`)
			return
		}
		http.NotFound(response, request)
	}))
	defer server.Close()

	manifest := githubToolManifest(server.URL, 7)
	client := NewClient("")
	client.HTTP = server.Client()
	_, err := UpdateManifest(t.Context(), &manifest, client)
	if err == nil || !strings.Contains(err.Error(), "reported no release date") {
		t.Fatalf("UpdateManifest() error = %v, want missing release date error", err)
	}
	if got := manifest.Tools[0].Version; got != "v1.0.0" {
		t.Fatalf("version = %q, want v1.0.0", got)
	}
}

func TestLatestVersionReportsReleaseDate(t *testing.T) {
	published := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name        string
		tool        Tool
		responses   map[string]string
		wantVersion string
	}{
		{
			name: "GitHub release",
			tool: Tool{Source: Source{Type: "github-release", Repository: "example/tool", LatestURL: "https://example.test/latest"}},
			responses: map[string]string{
				"https://example.test/latest": `{"tag_name":"v2.0.0","published_at":"2026-01-02T03:04:05Z"}`,
			},
			wantVersion: "v2.0.0",
		},
		{
			name: "HashiCorp checkpoint",
			tool: Tool{Source: Source{Type: "hashicorp-checkpoint", LatestURL: "https://example.test/checkpoint"}},
			responses: map[string]string{
				"https://example.test/checkpoint": fmt.Sprintf(`{"current_version":"1.14.9","current_release":%d}`, published.Unix()),
			},
			wantVersion: "1.14.9",
		},
		{
			name: "stable text resolved through its repository",
			tool: Tool{Source: Source{Type: "stable-text", Repository: "kubernetes/kubernetes", LatestURL: "https://example.test/stable.txt"}},
			responses: map[string]string{
				"https://example.test/stable.txt":                                          "v1.36.2\n",
				"https://api.github.com/repos/kubernetes/kubernetes/releases/tags/v1.36.2": `{"tag_name":"v1.36.2","published_at":"2026-01-02T03:04:05Z"}`,
			},
			wantVersion: "v1.36.2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewClient("")
			client.HTTP = httpClientFunc(func(request *http.Request) (*http.Response, error) {
				body, ok := test.responses[request.URL.String()]
				if !ok {
					return nil, fmt.Errorf("unexpected request for %s", request.URL)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})

			release, err := client.LatestVersion(t.Context(), test.tool)
			if err != nil {
				t.Fatalf("LatestVersion() error = %v", err)
			}
			if release.Version != test.wantVersion {
				t.Errorf("Version = %q, want %q", release.Version, test.wantVersion)
			}
			if !release.PublishedAt.Equal(published) {
				t.Errorf("PublishedAt = %s, want %s", release.PublishedAt, published)
			}
		})
	}
}

func githubToolManifest(serverURL string, cooldownDays int) Manifest {
	return Manifest{
		SchemaVersion: 1,
		CooldownDays:  cooldownDays,
		Platforms:     []string{"linux_amd64"},
		Tools: []Tool{{
			Name:       "tool",
			MakePrefix: "TOOL",
			Version:    "v1.0.0",
			Source: Source{
				Type:       "github-release",
				Repository: "example/tool",
				LatestURL:  serverURL + "/latest",
			},
			DownloadTemplate: serverURL + "/download/{tag}/{asset}",
			Platforms: map[string]Platform{
				"linux_amd64": {
					Asset:    "tool-{version}.tar.gz",
					Checksum: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				},
			},
			ChecksumSource: ChecksumSource{
				Type:        "url-file",
				URLTemplate: serverURL + "/checksums/{version}/{asset}",
				Format:      "standard",
			},
		}},
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

func TestClientOnlyAuthenticatesExactGitHubAPIHost(t *testing.T) {
	tests := []struct {
		name              string
		url               string
		wantAuthorization string
		wantAccept        string
	}{
		{
			name:              "GitHub API",
			url:               "https://api.github.com/repos/example/tool/releases/latest",
			wantAuthorization: "Bearer secret",
			wantAccept:        "application/vnd.github+json",
		},
		{
			name: "lookalike host",
			url:  "https://api.github.com.attacker.example/releases/latest",
		},
		{
			name: "GitHub release download",
			url:  "https://github.com/example/tool/releases/latest",
		},
		{
			name: "insecure GitHub API URL",
			url:  "http://api.github.com/repos/example/tool/releases/latest",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewClient("secret")
			client.HTTP = httpClientFunc(func(request *http.Request) (*http.Response, error) {
				if got := request.Header.Get("Authorization"); got != test.wantAuthorization {
					t.Errorf("Authorization = %q, want %q", got, test.wantAuthorization)
				}
				if got := request.Header.Get("Accept"); got != test.wantAccept {
					t.Errorf("Accept = %q, want %q", got, test.wantAccept)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("{}")),
				}, nil
			})
			if _, err := client.get(t.Context(), test.url); err != nil {
				t.Fatalf("get() error = %v", err)
			}
		})
	}
}

func TestClientStripsAuthorizationOnRedirectAwayFromGitHubAPI(t *testing.T) {
	client := NewClient("secret")
	httpClient, ok := client.HTTP.(*http.Client)
	if !ok {
		t.Fatalf("HTTP client has type %T, want *http.Client", client.HTTP)
	}
	request, err := http.NewRequest(http.MethodGet, "https://uploads.github.com/object", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer secret")
	if err := httpClient.CheckRedirect(request, nil); err != nil {
		t.Fatalf("CheckRedirect() error = %v", err)
	}
	if got := request.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization after redirect = %q, want empty", got)
	}
}

func TestClientLimitsRedirects(t *testing.T) {
	client := NewClient("secret")
	httpClient, ok := client.HTTP.(*http.Client)
	if !ok {
		t.Fatalf("HTTP client has type %T, want *http.Client", client.HTTP)
	}
	request, err := http.NewRequest(http.MethodGet, "https://api.github.com/redirect", nil)
	if err != nil {
		t.Fatal(err)
	}
	via := make([]*http.Request, 10)
	if err := httpClient.CheckRedirect(request, via); err == nil {
		t.Fatal("CheckRedirect() accepted more than 10 redirects")
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
		if _, err := client.Checksum(t.Context(), tool, platform, tool.Version); err != nil {
			t.Fatalf("Checksum(%s) error = %v", platform, err)
		}
	}
	if fetches != 1 {
		t.Fatalf("shared checksum file fetched %d times, want 1", fetches)
	}
}

func TestChecksumStreamsDownloadedAsset(t *testing.T) {
	const contents = "downloaded tool contents"
	want := fmt.Sprintf("%x", sha256.Sum256([]byte(contents)))

	var attempts int
	client := NewClient("")
	client.HTTP = httpClientFunc(func(request *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, fmt.Errorf("temporary download error")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(contents)),
		}, nil
	})
	tool := Tool{
		Version:          "v1.0.0",
		DownloadTemplate: "https://downloads.example.test/{asset}",
		Platforms: map[string]Platform{
			"linux_amd64": {Asset: "tool_linux_amd64"},
		},
		ChecksumSource: ChecksumSource{Type: "download"},
	}

	got, err := client.Checksum(t.Context(), tool, "linux_amd64", tool.Version)
	if err != nil {
		t.Fatalf("Checksum() error = %v", err)
	}
	if got != want {
		t.Fatalf("Checksum() = %q, want %q", got, want)
	}
	if attempts != 2 {
		t.Fatalf("download attempts = %d, want 2", attempts)
	}
}
