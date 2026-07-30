package tooling

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if request.URL.Scheme != "https" || request.URL.Hostname() != "api.github.com" {
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

// Release is the newest version advertised by a tool's source, with the time
// it was published when the source reports one.
type Release struct {
	Version     string
	PublishedAt time.Time
}

// UpdateResult reports the manifest values the updater changed and the newer
// releases it deferred because they are still inside the cooldown.
type UpdateResult struct {
	Changes []string
	Held    []string
}

// UpdateManifest refreshes versions and checksums in memory. It writes no
// files, so a failed source lookup cannot leave a partially updated manifest.
func UpdateManifest(ctx context.Context, manifest *Manifest, client *Client) (UpdateResult, error) {
	if err := manifest.Validate(); err != nil {
		return UpdateResult{}, err
	}
	if client == nil || client.HTTP == nil {
		return UpdateResult{}, fmt.Errorf("an HTTP client is required")
	}

	cooldown := manifest.Cooldown()
	now := time.Now()

	var result UpdateResult
	for index := range manifest.Tools {
		tool := &manifest.Tools[index]
		targetVersion := tool.Version
		latest, err := client.LatestVersion(ctx, *tool)
		if err != nil {
			return UpdateResult{}, fmt.Errorf("check %s version: %w", tool.Name, err)
		}
		newer, err := newerVersion(latest.Version, tool.Version)
		if err != nil {
			return UpdateResult{}, fmt.Errorf("compare %s versions: %w", tool.Name, err)
		}
		if newer && tool.UpdatesEnabled() {
			held, err := heldByCooldown(latest, cooldown, now)
			if err != nil {
				return UpdateResult{}, fmt.Errorf("check %s cooldown: %w", tool.Name, err)
			}
			if held {
				result.Held = append(result.Held, fmt.Sprintf("%s %s published %s, less than %d days ago",
					tool.Name, latest.Version, latest.PublishedAt.UTC().Format(time.DateOnly), manifest.CooldownDays))
			} else {
				result.Changes = append(result.Changes, fmt.Sprintf("%s version %s -> %s", tool.Name, tool.Version, latest.Version))
				targetVersion = latest.Version
			}
		}

		updatedChecksums := make(map[string]string, len(tool.Platforms))
		for _, platform := range manifest.Platforms {
			if len(tool.Platforms) == 0 {
				break
			}
			checksum, err := client.Checksum(ctx, *tool, platform, targetVersion)
			if err != nil {
				return UpdateResult{}, fmt.Errorf("check %s %s checksum: %w", tool.Name, platform, err)
			}
			updatedChecksums[platform] = checksum
			if checksum != tool.Platforms[platform].Checksum {
				result.Changes = append(result.Changes, fmt.Sprintf("%s %s checksum refreshed", tool.Name, platform))
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
	return result, nil
}

// heldByCooldown reports whether a candidate release is still too new to adopt.
// An unknown release date is an error rather than an implicit pass, so a source
// that stops reporting dates cannot silently bypass the cooldown.
func heldByCooldown(release Release, cooldown time.Duration, now time.Time) (bool, error) {
	if cooldown <= 0 {
		return false, nil
	}
	if release.PublishedAt.IsZero() {
		return false, fmt.Errorf("the source reported no release date for %s", release.Version)
	}
	return now.Sub(release.PublishedAt) < cooldown, nil
}

// LatestVersion resolves the latest stable release for a tool source.
func (client *Client) LatestVersion(ctx context.Context, tool Tool) (Release, error) {
	contents, err := client.get(ctx, tool.Source.LatestURL)
	if err != nil {
		return Release{}, err
	}

	switch tool.Source.Type {
	case "github-release":
		tag, publishedAt, err := parseGitHubRelease(contents)
		if err != nil {
			return Release{}, err
		}
		return Release{Version: tool.VersionFromTag(tag), PublishedAt: publishedAt}, nil
	case "stable-text":
		version := strings.TrimSpace(string(contents))
		if version == "" {
			return Release{}, fmt.Errorf("stable version response is empty")
		}
		if strings.TrimSpace(tool.Source.Repository) == "" {
			return Release{Version: version}, nil
		}
		publishedAt, err := client.releaseDate(ctx, tool, version)
		if err != nil {
			return Release{}, err
		}
		return Release{Version: version, PublishedAt: publishedAt}, nil
	case "hashicorp-checkpoint":
		var checkpoint struct {
			CurrentVersion string `json:"current_version"`
			CurrentRelease int64  `json:"current_release"`
		}
		if err := json.Unmarshal(contents, &checkpoint); err != nil {
			return Release{}, fmt.Errorf("parse HashiCorp checkpoint: %w", err)
		}
		if checkpoint.CurrentVersion == "" {
			return Release{}, fmt.Errorf("HashiCorp checkpoint has no current_version")
		}
		release := Release{Version: checkpoint.CurrentVersion}
		if checkpoint.CurrentRelease > 0 {
			release.PublishedAt = time.Unix(checkpoint.CurrentRelease, 0).UTC()
		}
		return release, nil
	default:
		return Release{}, fmt.Errorf("unsupported version source %q", tool.Source.Type)
	}
}

// releaseDate resolves when a version was published for a source that reports
// no timestamp of its own, using the GitHub release for the matching tag.
func (client *Client) releaseDate(ctx context.Context, tool Tool, version string) (time.Time, error) {
	releaseURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", tool.Source.Repository, tool.TagForVersion(version))
	contents, err := client.get(ctx, releaseURL)
	if err != nil {
		return time.Time{}, err
	}
	_, publishedAt, err := parseGitHubRelease(contents)
	return publishedAt, err
}

func parseGitHubRelease(contents []byte) (string, time.Time, error) {
	var release struct {
		TagName     string `json:"tag_name"`
		PublishedAt string `json:"published_at"`
	}
	if err := json.Unmarshal(contents, &release); err != nil {
		return "", time.Time{}, fmt.Errorf("parse GitHub release: %w", err)
	}
	if release.TagName == "" {
		return "", time.Time{}, fmt.Errorf("GitHub release has no tag")
	}
	if release.PublishedAt == "" {
		return release.TagName, time.Time{}, nil
	}
	publishedAt, err := time.Parse(time.RFC3339, release.PublishedAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parse GitHub release date %q: %w", release.PublishedAt, err)
	}
	return release.TagName, publishedAt, nil
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
		return client.hash(ctx, url)
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
	var contents bytes.Buffer
	if err := client.writeResponse(ctx, url, &contents); err != nil {
		return nil, err
	}
	return contents.Bytes(), nil
}

func (client *Client) hash(ctx context.Context, url string) (string, error) {
	hasher := sha256.New()
	if err := client.writeResponse(ctx, url, hasher); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (client *Client) writeResponse(ctx context.Context, url string, destination io.Writer) error {
	var lastError error
	for attempt := 0; attempt < requestAttempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create request for %s: %w", url, err)
		}
		request.Header.Set("User-Agent", client.UserAgent)
		isGitHubAPI := request.URL.Scheme == "https" && request.URL.Hostname() == "api.github.com"
		if isGitHubAPI {
			request.Header.Set("Accept", "application/vnd.github+json")
		}
		if client.Token != "" && isGitHubAPI {
			request.Header.Set("Authorization", "Bearer "+client.Token)
		}

		response, err := client.HTTP.Do(request)
		if err != nil {
			lastError = err
			continue
		}
		if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			// io.Copy can fail on either side, so the message stays neutral.
			written, copyErr := io.Copy(destination, io.LimitReader(response.Body, maximumResponse+1))
			closeErr := response.Body.Close()
			if copyErr != nil {
				return fmt.Errorf("copy response from %s: %w", url, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close response from %s: %w", url, closeErr)
			}
			if written > maximumResponse {
				return fmt.Errorf("response from %s exceeds %d bytes", url, maximumResponse)
			}
			return nil
		}

		contents, readErr := io.ReadAll(io.LimitReader(response.Body, maximumResponse+1))
		closeErr := response.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read %s: %w", url, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close response from %s: %w", url, closeErr)
		}
		if len(contents) > maximumResponse {
			return fmt.Errorf("response from %s exceeds %d bytes", url, maximumResponse)
		}
		lastError = fmt.Errorf("HTTP %s from %s: %s", response.Status, url, strings.TrimSpace(string(contents)))
		if response.StatusCode < 500 && response.StatusCode != http.StatusTooManyRequests {
			break
		}
		if attempt < requestAttempts-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
			}
		}
	}
	return lastError
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
