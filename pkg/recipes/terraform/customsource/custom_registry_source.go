/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package customsource provides a custom implementation for downloading Terraform
// binaries from private registries with custom TLS configuration.
//
// Note: This is a standalone implementation that doesn't integrate with hc-install
// due to its use of internal packages. Instead, it provides similar functionality
// that can be used as an alternative when custom TLS configuration is needed.
package customsource

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// HTTPClient is an interface for HTTP operations, allowing for easier testing
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// CustomRegistrySource implements a custom source for hc-install that supports
// private registries with custom TLS configuration
type CustomRegistrySource struct {
	// Product to install (e.g., product.Terraform)
	Product product.Product

	// Version to install
	Version *version.Version

	// BaseURL for the custom registry (e.g., "https://private-registry.com")
	BaseURL string

	// ArchiveURL is a direct URL to the Terraform archive (.zip file)
	// If set, this URL will be used directly instead of querying the registry API
	ArchiveURL string

	// InstallDir is the directory to install the product
	InstallDir string

	// AuthToken for authentication (e.g., "Bearer token" or "Basic base64(user:pass)")
	AuthToken string

	// CACertPEM is PEM-encoded CA certificate(s)
	CACertPEM []byte

	// InsecureSkipVerify skips TLS verification (not recommended for production)
	InsecureSkipVerify bool

	// Timeout for HTTP requests
	Timeout int

	// HTTPClient allows injection of a custom HTTP client (for testing)
	HTTPClient HTTPClient
}

// releaseIndex represents the structure of index.json from HashiCorp releases
type releaseIndex struct {
	Versions map[string]releaseVersion `json:"versions"`
}

type releaseVersion struct {
	Version string         `json:"version"`
	Builds  []releaseBuild `json:"builds"`
	Shasums string         `json:"shasums"`
}

type releaseBuild struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Filename string `json:"filename"`
	URL      string `json:"url"`
}

// Note: This implementation is standalone and doesn't integrate with hc-install's
// installer.Ensure() method due to the library's use of internal packages.

// Default retry configuration
const (
	retryMaxAttempts  = 3
	retryInitialDelay = 1 * time.Second
	retryMaxDelay     = 30 * time.Second
	retryMultiplier   = 2.0
)

// Validate checks if the source configuration is valid
func (s *CustomRegistrySource) Validate() error {
	logger := ucplog.FromContextOrDiscard(context.Background())
	logger.Info("Validating custom registry source configuration")

	if s.Product.Name == "" {
		logger.Error(nil, "Product name is required")
		return fmt.Errorf("product is required")
	}
	// Version is required only when using BaseURL (API-based installation)
	if s.ArchiveURL == "" && s.Version == nil {
		logger.Error(nil, "Version is required when not using direct archive URL")
		return fmt.Errorf("version is required when not using direct archive URL")
	}
	// Either BaseURL or ArchiveURL must be provided
	if s.BaseURL == "" && s.ArchiveURL == "" {
		logger.Error(nil, "Either BaseURL or ArchiveURL is required")
		return fmt.Errorf("either BaseURL or ArchiveURL is required")
	}
	if s.InstallDir == "" {
		logger.Error(nil, "Install directory is required")
		return fmt.Errorf("installDir is required")
	}

	logger.Info("Custom registry source validation passed",
		"product", s.Product.Name,
		"hasVersion", s.Version != nil,
		"hasArchiveURL", s.ArchiveURL != "",
		"hasBaseURL", s.BaseURL != "")
	return nil
}

// Install downloads and installs the product from the custom registry
func (s *CustomRegistrySource) Install(ctx context.Context) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	if err := s.Validate(); err != nil {
		return "", err
	}

	// Get HTTP client (either injected or create default)
	client, err := s.getHTTPClient()
	if err != nil {
		return "", fmt.Errorf("failed to get HTTP client: %w", err)
	}

	// Ensure install directory exists
	if err = os.MkdirAll(s.InstallDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create install directory: %w", err)
	}

	var zipPath string

	// Check if we have a direct archive URL
	if s.ArchiveURL != "" {
		// Direct download from archive URL
		logger.Info("Using direct archive URL", "url", s.ArchiveURL)

		// Extract filename from URL
		u, err := url.Parse(s.ArchiveURL)
		if err != nil {
			return "", fmt.Errorf("failed to parse archive URL: %w", err)
		}

		filename := filepath.Base(u.Path)
		if filename == "" || filename == "." || filename == "/" {
			return "", fmt.Errorf("archive URL must include a filename: %s", s.ArchiveURL)
		}

		logger.Info("Downloading Terraform archive", "filename", filename, "url", s.ArchiveURL)
		zipPath, err = s.downloadFile(ctx, client, s.ArchiveURL, filename)
		if err != nil {
			return "", fmt.Errorf("failed to download from archive URL: %w", err)
		}
		defer func() { _ = os.Remove(zipPath) }()
	} else {
		// Standard flow: fetch release info from API
		logger.Info("Fetching release information", "product", s.Product.Name, "version", s.Version)
		var releaseInfo *releaseVersion
		releaseInfo, err = s.fetchReleaseInfo(ctx, client)
		if err != nil {
			return "", fmt.Errorf("failed to fetch release info: %w", err)
		}

		// Find the build for current OS/arch
		build, err := s.findBuild(releaseInfo)
		if err != nil {
			return "", err
		}

		// Download the binary
		logger.Info("Downloading Terraform binary", "filename", build.Filename, "url", build.URL)
		zipPath, err = s.downloadFile(ctx, client, build.URL, build.Filename)
		if err != nil {
			return "", fmt.Errorf("failed to download: %w", err)
		}
		defer func() { _ = os.Remove(zipPath) }()

		// Download and verify checksums
		if releaseInfo.Shasums != "" {
			logger.Info("Downloading and verifying checksums")
			if err = s.verifyChecksum(ctx, client, releaseInfo.Shasums, zipPath, build.Filename); err != nil {
				return "", fmt.Errorf("checksum verification failed: %w", err)
			}
		}
	}

	// Extract the binary
	execPath, err := s.extractBinary(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to extract binary: %w", err)
	}

	// Make executable
	if err = os.Chmod(execPath, 0755); err != nil {
		return "", fmt.Errorf("failed to make executable: %w", err)
	}

	// Log success
	if s.Version != nil {
		logger.Info("Successfully installed Terraform", "product", s.Product.Name, "version", s.Version, "path", execPath)
	} else {
		logger.Info("Successfully installed Terraform", "product", s.Product.Name, "path", execPath)
	}
	return execPath, nil
}

// getHTTPClient returns the HTTP client to use - either injected or creates a default one
func (s *CustomRegistrySource) getHTTPClient() (HTTPClient, error) {
	logger := ucplog.FromContextOrDiscard(context.Background())

	// If an HTTP client was injected, use it
	if s.HTTPClient != nil {
		logger.Info("Using injected HTTP client")
		return s.HTTPClient, nil
	}

	// Otherwise create a default client with custom TLS configuration
	logger.Info("Creating default HTTP client with custom TLS configuration", "insecureSkipVerify", s.InsecureSkipVerify)
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: s.InsecureSkipVerify,
	}

	// Add custom CA if provided and not skipping verification
	if len(s.CACertPEM) > 0 && !s.InsecureSkipVerify {
		logger.Info("Configuring custom CA certificate")
		caCertPool, err := x509.SystemCertPool()
		if err != nil {
			// Fall back to empty pool if system certs unavailable
			logger.Info("Failed to load system cert pool, creating empty pool", "error", err.Error())
			caCertPool = x509.NewCertPool()
		}

		if !caCertPool.AppendCertsFromPEM(s.CACertPEM) {
			logger.Error(nil, "Failed to parse CA certificate")
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		logger.Info("Successfully added custom CA certificate to pool")

		tlsConfig.RootCAs = caCertPool
	}

	timeout := s.Timeout
	if timeout == 0 {
		timeout = 300 // 5 minutes default
		logger.Info("Using default timeout", "seconds", timeout)
	} else {
		logger.Info("Using configured timeout", "seconds", timeout)
	}

	return &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}, nil
}

// doWithRetry performs an HTTP request with retry logic
func (s *CustomRegistrySource) doWithRetry(ctx context.Context, client HTTPClient, req *http.Request) (*http.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	var lastErr error
	delay := retryInitialDelay

	for attempt := 0; attempt < retryMaxAttempts; attempt++ {
		// Clone request for retry
		reqClone := req.Clone(ctx)
		if req.Body != nil {
			return nil, fmt.Errorf("retry with request body not yet implemented")
		}

		// Check context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		resp, err := client.Do(reqClone)
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}

		// Handle error
		if err != nil {
			logger.Info("Request failed, retrying", "attempt", attempt+1, "maxAttempts", retryMaxAttempts, "error", err.Error())
			lastErr = err
		} else {
			logger.Info("Request failed with status, retrying", "status", resp.StatusCode, "attempt", attempt+1, "maxAttempts", retryMaxAttempts)
			lastErr = fmt.Errorf("server returned status %d", resp.StatusCode)
			_ = resp.Body.Close()
		}

		// Sleep before retry (except last attempt)
		if attempt < retryMaxAttempts-1 {
			logger.Info("Retrying after delay", "delay", delay.String())
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}

			// Exponential backoff
			delay = time.Duration(float64(delay) * retryMultiplier)
			if delay > retryMaxDelay {
				delay = retryMaxDelay
			}
		}
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", retryMaxAttempts, lastErr)
}

// buildURL constructs a URL by joining the base URL with the given path segments
func (s *CustomRegistrySource) buildURL(segments ...string) (string, error) {
	// Ensure base URL is valid
	if s.BaseURL == "" {
		return "", fmt.Errorf("base URL is empty")
	}

	// Parse to validate and normalize
	baseURL, err := url.Parse(s.BaseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	// Build path efficiently
	if len(segments) > 0 {
		// Combine base path with segments
		allSegments := make([]string, 0, len(segments)+1)
		if baseURL.Path != "" && baseURL.Path != "/" {
			allSegments = append(allSegments, baseURL.Path)
		}
		allSegments = append(allSegments, segments...)
		baseURL.Path = path.Join(allSegments...)
	}

	return baseURL.String(), nil
}

// fetchReleaseInfo fetches the release information from the custom registry
func (s *CustomRegistrySource) fetchReleaseInfo(ctx context.Context, client HTTPClient) (*releaseVersion, error) {
	// Build index URL following HashiCorp's structure
	indexURL, err := s.buildURL(s.Product.Name, "index.json")
	if err != nil {
		return nil, fmt.Errorf("failed to build index URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", indexURL, nil)
	if err != nil {
		return nil, err
	}

	if s.AuthToken != "" {
		req.Header.Set("Authorization", s.AuthToken)
	}

	resp, err := s.doWithRetry(ctx, client, req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch index.json: %s", resp.Status)
	}

	var index releaseIndex
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, fmt.Errorf("failed to parse index.json: %w", err)
	}

	// Find the requested version
	versionStr := s.Version.String()
	release, ok := index.Versions[versionStr]
	if !ok {
		return nil, fmt.Errorf("version %s not found in registry", versionStr)
	}

	// Return pointer to avoid copy
	return &release, nil
}

// findBuild finds the build for the current OS/architecture
func (s *CustomRegistrySource) findBuild(release *releaseVersion) (*releaseBuild, error) {
	currentOS := runtime.GOOS
	currentArch := runtime.GOARCH

	for _, build := range release.Builds {
		if build.OS == currentOS && build.Arch == currentArch {
			// Convert relative URL to absolute if needed
			if !strings.HasPrefix(build.URL, "http") {
				var err error
				build.URL, err = s.buildURL(s.Product.Name, s.Version.String(), build.Filename)
				if err != nil {
					return nil, fmt.Errorf("failed to build download URL: %w", err)
				}
			}
			return &build, nil
		}
	}

	return nil, fmt.Errorf("no build found for %s/%s", currentOS, currentArch)
}

// downloadFile downloads a file from the given URL with improved error handling and cleanup
func (s *CustomRegistrySource) downloadFile(ctx context.Context, client HTTPClient, downloadURL, filename string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return "", err
	}

	if s.AuthToken != "" {
		req.Header.Set("Authorization", s.AuthToken)
	}

	resp, err := s.doWithRetry(ctx, client, req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp(s.InstallDir, filename+".tmp-*")
	if err != nil {
		return "", err
	}

	// Capture path before defer
	tempPath := tmpFile.Name()
	cleanupTemp := true

	defer func() {
		_ = tmpFile.Close()
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	// Copy content directly - io.Copy already handles interruption well
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}

	// Download successful, don't cleanup temp file
	cleanupTemp = false
	return tempPath, nil
}

// verifyChecksum downloads and verifies the SHA256 checksum
func (s *CustomRegistrySource) verifyChecksum(ctx context.Context, client HTTPClient, shasumsURL, filePath, filename string) error {
	// Build shasums URL if relative
	if !strings.HasPrefix(shasumsURL, "http") {
		var err error
		shasumsURL, err = s.buildURL(s.Product.Name, s.Version.String(), shasumsURL)
		if err != nil {
			return fmt.Errorf("failed to build shasums URL: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", shasumsURL, nil)
	if err != nil {
		return err
	}

	if s.AuthToken != "" {
		req.Header.Set("Authorization", s.AuthToken)
	}

	resp, err := s.doWithRetry(ctx, client, req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download shasums: %s", resp.Status)
	}

	// Stream and parse shasums content
	var expectedSum string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == filename {
			expectedSum = parts[0]
			break
		}
	}

	if err = scanner.Err(); err != nil {
		return fmt.Errorf("error reading shasums: %w", err)
	}

	if expectedSum == "" {
		return fmt.Errorf("checksum not found for %s", filename)
	}

	// Calculate actual checksum efficiently
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	// Use buffered reader for better performance on large files
	h := sha256.New()
	if _, err := io.CopyBuffer(h, f, make([]byte, 64*1024)); err != nil {
		return err
	}

	actualSum := hex.EncodeToString(h.Sum(nil))

	if actualSum != expectedSum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedSum, actualSum)
	}

	return nil
}

// extractBinary extracts the binary from the zip file
func (s *CustomRegistrySource) extractBinary(zipPath string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()

	// Look for the executable
	execName := s.Product.Name
	if runtime.GOOS == "windows" {
		execName += ".exe"
	}

	for _, f := range r.File {
		if f.Name == execName {
			return s.extractFile(f)
		}
	}

	return "", fmt.Errorf("executable %s not found in archive", execName)
}

// extractFile extracts a single file from the zip
func (s *CustomRegistrySource) extractFile(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	// Validate and clean the file name to prevent directory traversal
	cleanName := filepath.Base(f.Name)
	if cleanName != f.Name || strings.ContainsAny(cleanName, "/\\") {
		return "", fmt.Errorf("invalid file path in archive: %s", f.Name)
	}

	// Construct the destination path
	execPath := filepath.Join(s.InstallDir, cleanName)

	// Verify the path is within InstallDir (single check)
	absInstallDir, err := filepath.Abs(s.InstallDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute install directory: %w", err)
	}

	absExecPath, err := filepath.Abs(execPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute exec path: %w", err)
	}

	if !strings.HasPrefix(absExecPath, absInstallDir+string(filepath.Separator)) && absExecPath != absInstallDir {
		return "", fmt.Errorf("invalid destination path: %s", execPath)
	}

	outFile, err := os.OpenFile(execPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return "", err
	}
	defer func() { _ = outFile.Close() }()

	// Use buffered copy for better performance
	if _, err := io.CopyBuffer(outFile, rc, make([]byte, 32*1024)); err != nil {
		return "", err
	}

	return execPath, nil
}
