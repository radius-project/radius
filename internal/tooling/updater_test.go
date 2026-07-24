package tooling

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

	manifest := Manifest{
		SchemaVersion: 1,
		Platforms:     []string{"linux_amd64"},
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
			if _, err := client.get(context.Background(), test.url); err != nil {
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
		if _, err := client.Checksum(context.Background(), tool, platform, tool.Version); err != nil {
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

	got, err := client.Checksum(context.Background(), tool, "linux_amd64", tool.Version)
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
