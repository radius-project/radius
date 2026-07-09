package tooling

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
)

const (
	requestAttempts = 3
	maximumResponse = 128 << 20
)

// HTTPClient is the subset of http.Client used by the updater.
type HTTPClient interface {
	Do(request *http.Request) (*http.Response, error)
}

// Client retrieves release metadata and checksum sources.
type Client struct {
	HTTP      HTTPClient
	Token     string
	UserAgent string
	fileCache map[string][]byte
}

// NewClient constructs a source client. GITHUB_TOKEN and GH_TOKEN are honored
// to keep local and GitHub Actions runs equivalent.
func NewClient(token string) *Client {
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	httpClient := &http.Client{
		Timeout: 90 * time.Second,
		CheckRedirect: func(request *http.Request, _ []*http.Request) error {
			if request.URL.Hostname() != "api.github.com" {
				request.Header.Del("Authorization")
			}
			return nil
		},
	}
	return &Client{
		HTTP:      httpClient,
		Token:     token,
		UserAgent: "radius-tool-updater",
		fileCache: map[string][]byte{},
	}
}

// UpdateManifest refreshes versions and checksums in memory. It writes no
// files, so a failed source lookup cannot leave a partially updated manifest.
func UpdateManifest(ctx context.Context, manifest *Manifest, client *Client) ([]string, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	if client == nil || client.HTTP == nil {
		return nil, fmt.Errorf("an HTTP client is required")
	}

	var changes []string
	for index := range manifest.Tools {
		tool := &manifest.Tools[index]
		targetVersion := tool.Version
		latest, err := client.LatestVersion(ctx, *tool)
		if err != nil {
			return nil, fmt.Errorf("check %s version: %w", tool.Name, err)
		}
		newer, err := newerVersion(latest, tool.Version)
		if err != nil {
			return nil, fmt.Errorf("compare %s versions: %w", tool.Name, err)
		}
		if newer && tool.UpdatesEnabled() {
			changes = append(changes, fmt.Sprintf("%s version %s -> %s", tool.Name, tool.Version, latest))
			targetVersion = latest
		}

		updatedChecksums := make(map[string]string, len(tool.Platforms))
		for _, platform := range manifest.Platforms {
			if len(tool.Platforms) == 0 {
				break
			}
			checksum, err := client.Checksum(ctx, *tool, platform, targetVersion)
			if err != nil {
				return nil, fmt.Errorf("check %s %s checksum: %w", tool.Name, platform, err)
			}
			updatedChecksums[platform] = checksum
			if checksum != tool.Platforms[platform].Checksum {
				changes = append(changes, fmt.Sprintf("%s %s checksum refreshed", tool.Name, platform))
			}
		}

		if targetVersion != tool.Version {
			tool.Version = targetVersion
		}
		for platform, checksum := range updatedChecksums {
			entry := tool.Platforms[platform]
			entry.Checksum = checksum
			tool.Platforms[platform] = entry
		}
	}
	return changes, nil
}

// LatestVersion resolves the latest stable version for a tool source.
func (client *Client) LatestVersion(ctx context.Context, tool Tool) (string, error) {
	contents, err := client.get(ctx, tool.Source.LatestURL)
	if err != nil {
		return "", err
	}

	switch tool.Source.Type {
	case "github-release":
		var release struct {
			TagName string `json:"tag_name"`
		}
		if err := json.Unmarshal(contents, &release); err != nil {
			return "", fmt.Errorf("parse GitHub release: %w", err)
		}
		if release.TagName == "" {
			return "", fmt.Errorf("GitHub release has no tag")
		}
		return tool.VersionFromTag(release.TagName), nil
	case "stable-text":
		version := strings.TrimSpace(string(contents))
		if version == "" {
			return "", fmt.Errorf("stable version response is empty")
		}
		return version, nil
	case "hashicorp-checkpoint":
		var checkpoint struct {
			CurrentVersion string `json:"current_version"`
		}
		if err := json.Unmarshal(contents, &checkpoint); err != nil {
			return "", fmt.Errorf("parse HashiCorp checkpoint: %w", err)
		}
		if checkpoint.CurrentVersion == "" {
			return "", fmt.Errorf("HashiCorp checkpoint has no current_version")
		}
		return checkpoint.CurrentVersion, nil
	default:
		return "", fmt.Errorf("unsupported version source %q", tool.Source.Type)
	}
}

// Checksum reads or computes the checksum for one target platform.
func (client *Client) Checksum(ctx context.Context, tool Tool, platform, version string) (string, error) {
	values, err := tool.TemplateValues(platform, version)
	if err != nil {
		return "", err
	}
	asset, err := ExpandTemplate(values["asset"], values)
	if err != nil {
		return "", fmt.Errorf("expand asset: %w", err)
	}
	values["asset"] = asset

	switch tool.ChecksumSource.Type {
	case "github-release-file":
		file, err := ExpandTemplate(tool.ChecksumSource.FileTemplate, values)
		if err != nil {
			return "", fmt.Errorf("expand checksum file: %w", err)
		}
		fileURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", tool.Source.Repository, values["tag"], file)
		contents, err := client.getFile(ctx, fileURL)
		if err != nil {
			return "", err
		}
		if tool.ChecksumSource.Format == "yq" {
			orderFile, err := ExpandTemplate(tool.ChecksumSource.OrderFileTemplate, values)
			if err != nil {
				return "", fmt.Errorf("expand checksum order file: %w", err)
			}
			orderURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", tool.Source.Repository, values["tag"], orderFile)
			orderContents, err := client.getFile(ctx, orderURL)
			if err != nil {
				return "", err
			}
			return parseYQChecksum(orderContents, contents, asset)
		}
		return parseChecksum(contents, tool.ChecksumSource.Format, asset)
	case "url-file":
		url, err := ExpandTemplate(tool.ChecksumSource.URLTemplate, values)
		if err != nil {
			return "", fmt.Errorf("expand checksum URL: %w", err)
		}
		contents, err := client.getFile(ctx, url)
		if err != nil {
			return "", err
		}
		return parseChecksum(contents, tool.ChecksumSource.Format, asset)
	case "download":
		url, err := ExpandTemplate(tool.DownloadTemplate, values)
		if err != nil {
			return "", fmt.Errorf("expand download URL: %w", err)
		}
		contents, err := client.get(ctx, url)
		if err != nil {
			return "", err
		}
		digest := sha256.Sum256(contents)
		return hex.EncodeToString(digest[:]), nil
	case "none":
		return "", nil
	default:
		return "", fmt.Errorf("unsupported checksum source %q", tool.ChecksumSource.Type)
	}
}

// getFile fetches a small checksum metadata file, caching it for the lifetime
// of the client so a shared checksums file is fetched once per run instead of
// once per platform. Binary downloads (the "download" checksum source) bypass
// this cache to avoid holding large blobs in memory.
func (client *Client) getFile(ctx context.Context, url string) ([]byte, error) {
	if contents, ok := client.fileCache[url]; ok {
		return contents, nil
	}
	contents, err := client.get(ctx, url)
	if err != nil {
		return nil, err
	}
	if client.fileCache != nil {
		client.fileCache[url] = contents
	}
	return contents, nil
}

func (client *Client) get(ctx context.Context, url string) ([]byte, error) {
	var lastError error
	for attempt := 0; attempt < requestAttempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("create request for %s: %w", url, err)
		}
		request.Header.Set("User-Agent", client.UserAgent)
		if request.URL.Hostname() == "api.github.com" {
			request.Header.Set("Accept", "application/vnd.github+json")
		}
		if client.Token != "" && request.URL.Hostname() == "api.github.com" {
			request.Header.Set("Authorization", "Bearer "+client.Token)
		}

		response, err := client.HTTP.Do(request)
		if err != nil {
			lastError = err
			continue
		}
		contents, readErr := io.ReadAll(io.LimitReader(response.Body, maximumResponse+1))
		closeErr := response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", url, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close response from %s: %w", url, closeErr)
		}
		if len(contents) > maximumResponse {
			return nil, fmt.Errorf("response from %s exceeds %d bytes", url, maximumResponse)
		}
		if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			return contents, nil
		}
		lastError = fmt.Errorf("HTTP %s from %s: %s", response.Status, url, strings.TrimSpace(string(contents)))
		if response.StatusCode < 500 && response.StatusCode != http.StatusTooManyRequests {
			break
		}
		if attempt < requestAttempts-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
			}
		}
	}
	return nil, lastError
}

func newerVersion(candidate, current string) (bool, error) {
	candidateVersion, err := semver.NewVersion(strings.TrimPrefix(candidate, "v"))
	if err != nil {
		return false, fmt.Errorf("parse candidate %q: %w", candidate, err)
	}
	currentVersion, err := semver.NewVersion(strings.TrimPrefix(current, "v"))
	if err != nil {
		return false, fmt.Errorf("parse current %q: %w", current, err)
	}
	return candidateVersion.GreaterThan(currentVersion), nil
}

func parseChecksum(contents []byte, format, asset string) (string, error) {
	lines := strings.Split(string(contents), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if format == "first" {
			return validateHash(fields[0])
		}
		if len(fields) < 2 {
			continue
		}
		filename := strings.TrimPrefix(fields[1], "*")
		if format == "basename" {
			filename = path.Base(filename)
		}
		if filename == asset {
			return validateHash(fields[0])
		}
	}
	return "", fmt.Errorf("checksum for %s not found", asset)
}

func parseYQChecksum(orderContents, checksumContents []byte, asset string) (string, error) {
	orderLines := strings.Split(string(orderContents), "\n")
	column := 0
	for index, line := range orderLines {
		if strings.TrimSpace(line) == "SHA-256" {
			column = index + 1
			break
		}
	}
	if column == 0 {
		return "", fmt.Errorf("SHA-256 column not found in checksums_hashes_order")
	}

	for _, line := range strings.Split(string(checksumContents), "\n") {
		fields := strings.Fields(line)
		if len(fields) > column && fields[0] == asset {
			return validateHash(fields[column])
		}
	}
	return "", fmt.Errorf("checksum for %s not found", asset)
}

func validateHash(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != sha256.Size*2 {
		return "", fmt.Errorf("invalid SHA-256 value %q", value)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", fmt.Errorf("invalid SHA-256 value %q: %w", value, err)
	}
	return value, nil
}
