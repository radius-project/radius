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
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
)

// HTTPClient is an interface for HTTP operations, allowing for easier testing
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// RetryConfig configures retry behavior for network operations
type RetryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
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

	// InstallDir is the directory to install the product
	InstallDir string

	// AuthToken for authentication (e.g., "Bearer token" or "Basic base64(user:pass)")
	AuthToken string

	// CACertPEM is PEM-encoded CA certificate(s)
	CACertPEM []byte

	// ClientCertPEM is PEM-encoded client certificate for mTLS
	ClientCertPEM []byte

	// ClientKeyPEM is PEM-encoded client key for mTLS
	ClientKeyPEM []byte

	// InsecureSkipVerify skips TLS verification (not recommended for production)
	InsecureSkipVerify bool

	// Timeout for HTTP requests
	Timeout int

	// RetryConfig for network operations (optional, uses defaults if nil)
	RetryConfig *RetryConfig

	// HTTPClient allows injection of a custom HTTP client (for testing)
	HTTPClient HTTPClient

	// logger for debug output
	logger *log.Logger

	// pathsToRemove tracks files to clean up
	pathsToRemove []string
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

// DefaultRetryConfig returns the default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
}

// Validate checks if the source configuration is valid
func (s *CustomRegistrySource) Validate() error {
	if s.Product.Name == "" {
		return fmt.Errorf("Product is required")
	}
	if s.Version == nil {
		return fmt.Errorf("Version is required")
	}
	if s.BaseURL == "" {
		return fmt.Errorf("BaseURL is required")
	}
	if s.InstallDir == "" {
		return fmt.Errorf("InstallDir is required")
	}
	return nil
}

// SetLogger sets the logger for debug output
func (s *CustomRegistrySource) SetLogger(logger *log.Logger) {
	s.logger = logger
}

// log writes to the logger if available
func (s *CustomRegistrySource) log(format string, v ...interface{}) {
	if s.logger != nil {
		s.logger.Printf(format, v...)
	}
}

// Install downloads and installs the product from the custom registry
func (s *CustomRegistrySource) Install(ctx context.Context) (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}

	// Get HTTP client (either injected or create default)
	client, err := s.getHTTPClient()
	if err != nil {
		return "", fmt.Errorf("failed to get HTTP client: %w", err)
	}

	// Ensure install directory exists
	if err := os.MkdirAll(s.InstallDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create install directory: %w", err)
	}

	// Get release information
	s.log("Fetching release information for %s %s", s.Product.Name, s.Version)
	releaseInfo, err := s.fetchReleaseInfo(ctx, client)
	if err != nil {
		return "", fmt.Errorf("failed to fetch release info: %w", err)
	}

	// Find the build for current OS/arch
	build, err := s.findBuild(releaseInfo)
	if err != nil {
		return "", err
	}

	// Download the binary
	s.log("Downloading %s from %s", build.Filename, build.URL)
	zipPath, err := s.downloadFile(ctx, client, build.URL, build.Filename)
	if err != nil {
		return "", fmt.Errorf("failed to download: %w", err)
	}
	s.pathsToRemove = append(s.pathsToRemove, zipPath)

	// Download and verify checksums
	if releaseInfo.Shasums != "" {
		s.log("Downloading and verifying checksums")
		if err := s.verifyChecksum(ctx, client, releaseInfo.Shasums, zipPath, build.Filename); err != nil {
			return "", fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	// Extract the binary
	s.log("Extracting %s", build.Filename)
	execPath, err := s.extractBinary(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to extract binary: %w", err)
	}

	// Make executable
	if err := os.Chmod(execPath, 0755); err != nil {
		return "", fmt.Errorf("failed to make executable: %w", err)
	}

	s.log("Successfully installed %s %s to %s", s.Product.Name, s.Version, execPath)
	return execPath, nil
}

// getHTTPClient returns the HTTP client to use - either injected or creates a default one
func (s *CustomRegistrySource) getHTTPClient() (HTTPClient, error) {
	// If an HTTP client was injected, use it
	if s.HTTPClient != nil {
		return s.HTTPClient, nil
	}

	// Otherwise create a default client with custom TLS configuration
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: s.InsecureSkipVerify,
	}

	// Add custom CA if provided and not skipping verification
	if len(s.CACertPEM) > 0 && !s.InsecureSkipVerify {
		caCertPool, err := x509.SystemCertPool()
		if err != nil {
			// Fall back to empty pool if system certs unavailable
			caCertPool = x509.NewCertPool()
		}

		if !caCertPool.AppendCertsFromPEM(s.CACertPEM) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		tlsConfig.RootCAs = caCertPool
	}

	timeout := s.Timeout
	if timeout == 0 {
		timeout = 300 // 5 minutes default
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
	config := s.RetryConfig
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr error
	delay := config.InitialDelay

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		// Clone the request for each attempt
		reqClone := req.Clone(ctx)
		if req.Body != nil {
			// If there's a body, we need to handle it properly
			// For now, we'll just error as our current use cases don't have request bodies
			return nil, fmt.Errorf("retry with request body not yet implemented")
		}

		// Check context before making request
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err := client.Do(reqClone)
		if err == nil && resp.StatusCode < 500 {
			// Success or client error - don't retry
			return resp, nil
		}

		// Log the error
		if err != nil {
			s.log("Request failed (attempt %d/%d): %v", attempt+1, config.MaxAttempts, err)
			lastErr = err
		} else {
			s.log("Request failed with status %d (attempt %d/%d)", resp.StatusCode, attempt+1, config.MaxAttempts)
			lastErr = fmt.Errorf("server returned status %d", resp.StatusCode)
			resp.Body.Close()
		}

		// Don't sleep after the last attempt
		if attempt < config.MaxAttempts-1 {
			s.log("Retrying after %v", delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			// Calculate next delay with exponential backoff
			delay = time.Duration(float64(delay) * config.Multiplier)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		}
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", config.MaxAttempts, lastErr)
}

// buildURL constructs a URL by joining the base URL with the given path segments
func (s *CustomRegistrySource) buildURL(segments ...string) (string, error) {
	baseURL, err := url.Parse(s.BaseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	// Join path segments
	pathSegments := []string{baseURL.Path}
	pathSegments = append(pathSegments, segments...)
	baseURL.Path = path.Join(pathSegments...)

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp(s.InstallDir, filename+".tmp-*")
	if err != nil {
		return "", err
	}

	// Use defer with a variable to control cleanup
	var cleanupTemp bool = true
	tempPath := tmpFile.Name()

	defer func() {
		tmpFile.Close()
		if cleanupTemp {
			os.Remove(tempPath)
		}
	}()

	// Copy content with context cancellation support
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(tmpFile, resp.Body)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return "", fmt.Errorf("failed to download file: %w", err)
		}
	case <-ctx.Done():
		return "", ctx.Err()
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download shasums: %s", resp.Status)
	}

	// Read shasums content
	shasums, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Find the checksum for our file
	var expectedSum string
	for _, line := range strings.Split(string(shasums), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == filename {
			expectedSum = parts[0]
			break
		}
	}

	if expectedSum == "" {
		return fmt.Errorf("checksum not found for %s", filename)
	}

	// Calculate actual checksum
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
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
	defer r.Close()

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
	defer rc.Close()

	execPath := filepath.Join(s.InstallDir, f.Name)
	outFile, err := os.OpenFile(execPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, rc); err != nil {
		return "", err
	}

	return execPath, nil
}

// Remove cleans up any temporary files created during installation
func (s *CustomRegistrySource) Remove(ctx context.Context) error {
	var lastErr error
	for _, path := range s.pathsToRemove {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			lastErr = err
		}
	}
	return lastErr
}
