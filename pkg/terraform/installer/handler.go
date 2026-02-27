/*
Copyright 2026 The Radius Authors.

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

package installer

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/components/queue"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// DefaultDownloadTimeout is the default timeout for downloading Terraform binaries.
	// This is generous to accommodate large binaries on slow connections.
	DefaultDownloadTimeout = 30 * time.Minute

	// MaxDecompressedSize is the maximum allowed size for decompressed files (500MB).
	// This protects against ZIP bomb attacks where a small compressed file expands
	// to consume all available disk space.
	MaxDecompressedSize = 500 * 1024 * 1024
)

var (
	// ErrInstallerBusy indicates another installer operation is already running.
	ErrInstallerBusy = errors.New("installer is busy; another operation is in progress")

	// zipMagic is the magic bytes at the start of a ZIP file (PK\x03\x04).
	zipMagic = []byte{0x50, 0x4B, 0x03, 0x04}
)

// Handler processes installer queue messages.
type Handler struct {
	StatusStore StatusStore
	RootPath    string
	HTTPClient  *http.Client
	// BaseURL optionally overrides the default Terraform releases base URL (for mirrors/air-gapped).
	BaseURL string
	// ExecutionChecker checks if Terraform executions are in progress before uninstall.
	// If nil, the safety check is skipped (for testing or when not required).
	ExecutionChecker ExecutionChecker
}

// Handle processes a queue message.
func (h *Handler) Handle(ctx context.Context, msg *queue.Message) error {
	payload := &JobMessage{}
	if err := json.Unmarshal(msg.Data, payload); err != nil {
		return fmt.Errorf("failed to decode installer job: %w", err)
	}

	// Track queue state: decrement pending, set in-progress
	inProgress := fmt.Sprintf("%s:%s", payload.Operation, payload.Version)
	h.updateQueueState(ctx, inProgress)
	defer h.clearQueueInProgress(ctx)

	switch payload.Operation {
	case OperationInstall:
		return h.handleInstall(ctx, payload)
	case OperationUninstall:
		return h.handleUninstall(ctx, payload)
	default:
		return fmt.Errorf("unsupported installer operation: %s", payload.Operation)
	}
}

func (h *Handler) handleInstall(ctx context.Context, job *JobMessage) error {
	log := ucplog.FromContextOrDiscard(ctx)

	if err := h.ensureRoot(); err != nil {
		return err
	}
	lockFile, err := h.acquireLock()
	if err != nil {
		log.Error(err, "installer lock acquisition failed")
		return err
	}
	defer h.releaseLock(log, lockFile)

	start := time.Now()

	status, err := h.getOrInitStatus(ctx)
	if err != nil {
		return err
	}

	version, sourceURL, err := h.resolveInstallInputs(ctx, status, job)
	if err != nil {
		return err
	}
	job.Version = version
	if _, ok := status.Versions[""]; ok {
		log.Info("removing unexpected empty version entry from installer status")
		delete(status.Versions, "")
	}

	// Idempotency check: skip re-download if version is already installed and binary exists.
	// This check must come AFTER version is finalized.
	if vs, ok := status.Versions[job.Version]; ok && vs.State == VersionStateSucceeded {
		binaryPath := h.versionBinaryPath(job.Version)
		if _, err := os.Stat(binaryPath); err == nil {
			// If already the current version, nothing to do
			if status.Current == job.Version {
				log.Info("version already installed and active, skipping", "version", job.Version)
				return nil
			}
			// Version is installed but not current - skip download, just promote
			log.Info("version already installed, promoting to current", "version", job.Version)
			return h.promoteVersion(ctx, log, status, job.Version, binaryPath, start)
		}
		// Binary missing - continue with reinstall
		log.Info("version marked installed but binary missing, reinstalling", "version", job.Version)
	}

	// NOW initialize version status with finalized version and resolved sourceURL
	vs := status.Versions[job.Version]
	vs.Version = job.Version
	vs.SourceURL = sourceURL // Use resolved sourceURL, not job.SourceURL
	vs.Checksum = job.Checksum
	vs.State = VersionStateInstalling
	vs.LastError = ""
	if vs.Health == "" {
		vs.Health = HealthUnknown
	}
	status.Versions[job.Version] = vs
	if err := h.persistStatus(ctx, status); err != nil {
		return err
	}

	targetDir := h.versionDir(job.Version)
	if err := os.MkdirAll(targetDir, 0o750); err != nil {
		return fmt.Errorf("failed to create target dir: %w", err)
	}

	archivePath := h.versionArchivePath(job.Version)
	dlOpts := &downloadOptions{
		URL:        sourceURL,
		Dst:        archivePath,
		Checksum:   job.Checksum,
		CABundle:   job.CABundle,
		AuthHeader: job.AuthHeader,
		ClientCert: job.ClientCert,
		ClientKey:  job.ClientKey,
		ProxyURL:   job.ProxyURL,
	}
	if err := h.download(ctx, dlOpts); err != nil {
		_ = h.recordFailure(ctx, status, job.Version, err)
		return err
	}

	binaryPath := h.versionBinaryPath(job.Version)
	if err := h.stageBinary(ctx, archivePath, binaryPath); err != nil {
		_ = h.recordFailure(ctx, status, job.Version, err)
		return err
	}

	// Clean up downloaded archive to save disk space.
	if err := os.Remove(archivePath); err != nil && !os.IsNotExist(err) {
		log.V(1).Info("failed to remove download archive", "path", archivePath, "error", err)
	}

	return h.promoteVersion(ctx, log, status, job.Version, binaryPath, start)
}

// resolveInstallInputs normalizes version/sourceURL inputs and validates the version for path safety.
func (h *Handler) resolveInstallInputs(ctx context.Context, status *Status, job *JobMessage) (string, string, error) {
	version := strings.TrimSpace(job.Version)
	sourceURL := strings.TrimSpace(job.SourceURL)
	if sourceURL == "" {
		// Version-only install: require version and build default URL.
		if version == "" {
			return "", "", errors.New("version or sourceUrl is required")
		}
		if err := ValidateVersionForPath(version); err != nil {
			_ = h.recordFailure(ctx, status, version, err)
			return "", "", err
		}
		sourceURL = h.defaultTerraformURL(version)
	} else {
		// SourceURL provided: generate version from URL hash if not specified.
		if version == "" {
			version = generateVersionFromURL(sourceURL)
		}
		if err := ValidateVersionForPath(version); err != nil {
			_ = h.recordFailure(ctx, status, version, err)
			return "", "", err
		}
	}

	return version, sourceURL, nil
}

// promoteVersion updates status to mark a version as current and updates the symlink.
// This is called both after a fresh download and when promoting an already-installed version.
func (h *Handler) promoteVersion(ctx context.Context, log logr.Logger, status *Status, version, binaryPath string, start time.Time) error {
	vs := status.Versions[version]
	vs.State = VersionStateSucceeded
	vs.Health = HealthHealthy
	vs.InstalledAt = time.Now().UTC()
	status.Previous = status.Current
	status.Current = version
	status.Versions[version] = vs
	status.LastError = ""

	if err := h.updateCurrentSymlink(binaryPath); err != nil {
		return err
	}
	if err := h.persistStatus(ctx, status); err != nil {
		return err
	}

	log.Info("promoted terraform version", "version", version, "path", binaryPath, "duration", time.Since(start))
	return nil
}

func (h *Handler) handleUninstall(ctx context.Context, job *JobMessage) error {
	log := ucplog.FromContextOrDiscard(ctx)

	if err := h.ensureRoot(); err != nil {
		return err
	}
	lockFile, err := h.acquireLock()
	if err != nil {
		log.Error(err, "installer lock acquisition failed")
		return err
	}
	defer h.releaseLock(log, lockFile)

	start := time.Now()

	status, err := h.getOrInitStatus(ctx)
	if err != nil {
		return err
	}

	// Validate version before using it in filesystem paths to prevent path traversal attacks.
	if err := ValidateVersionForPath(job.Version); err != nil {
		return err
	}

	vs, ok := status.Versions[job.Version]
	if !ok {
		return fmt.Errorf("version %s not found", job.Version)
	}

	// If purging an already-uninstalled version, just delete the metadata
	if job.Purge && (vs.State == VersionStateUninstalled || vs.State == VersionStateFailed) {
		delete(status.Versions, job.Version)
		if err := h.persistStatus(ctx, status); err != nil {
			return err
		}
		log.Info("purged terraform version metadata", "version", job.Version)
		return nil
	}

	// Safety check: ensure no Terraform executions are in progress before uninstalling.
	if h.ExecutionChecker != nil {
		active, err := h.ExecutionChecker.HasActiveExecutions(ctx)
		if err != nil {
			return fmt.Errorf("failed to check active executions: %w", err)
		}
		if active {
			return fmt.Errorf("cannot uninstall: Terraform executions are in progress")
		}
	}

	// Handle uninstalling the current version: switch to previous or clear.
	if status.Current == job.Version {
		if status.Previous != "" {
			// Verify previous version binary exists before switching.
			prevBinary := h.versionBinaryPath(status.Previous)
			if _, err := os.Stat(prevBinary); err != nil {
				// Previous version binary missing - update its state and clear current.
				if prevVS, ok := status.Versions[status.Previous]; ok {
					prevVS.State = VersionStateFailed
					prevVS.LastError = "binary not found during version switch"
					status.Versions[status.Previous] = prevVS
				}
				status.Current = ""
				status.Previous = ""
				// Remove current symlink.
				_ = os.Remove(h.currentSymlinkPath())
			} else {
				// Switch to previous version.
				status.Current = status.Previous
				status.Previous = ""
				if err := h.updateCurrentSymlink(prevBinary); err != nil {
					return fmt.Errorf("failed to switch to previous version: %w", err)
				}
			}
		} else {
			// No previous version, clear current.
			status.Current = ""
			// Remove current symlink.
			_ = os.Remove(h.currentSymlinkPath())
		}
		if err := h.persistStatus(ctx, status); err != nil {
			return err
		}
	}

	vs.State = VersionStateUninstalling
	status.Versions[job.Version] = vs
	if err := h.persistStatus(ctx, status); err != nil {
		return err
	}

	targetDir := h.versionDir(job.Version)
	if err := os.RemoveAll(targetDir); err != nil {
		_ = h.recordFailure(ctx, status, job.Version, err)
		return err
	}

	// If purge is requested, remove the version entry entirely from metadata
	if job.Purge {
		delete(status.Versions, job.Version)
		if err := h.persistStatus(ctx, status); err != nil {
			return err
		}
		log.Info("purged terraform version", "version", job.Version, "path", targetDir, "duration", time.Since(start))
		return nil
	}

	// Otherwise, mark as uninstalled but keep metadata for audit
	vs.State = VersionStateUninstalled
	vs.Health = HealthUnknown
	vs.LastError = ""
	status.Versions[job.Version] = vs
	if err := h.persistStatus(ctx, status); err != nil {
		return err
	}

	log.Info("uninstalled terraform", "version", job.Version, "path", targetDir, "duration", time.Since(start))
	return nil
}

// downloadOptions contains all options for downloading a file.
type downloadOptions struct {
	URL        string
	Dst        string
	Checksum   string
	CABundle   string
	AuthHeader string
	ClientCert string
	ClientKey  string
	ProxyURL   string
}

func (h *Handler) download(ctx context.Context, opts *downloadOptions) error {
	log := ucplog.FromContextOrDiscard(ctx)

	// Validate URL scheme to prevent file://, ftp://, or other potentially dangerous schemes
	parsedURL, err := url.Parse(opts.URL)
	if err != nil {
		return fmt.Errorf("invalid download URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("download URL must use http or https scheme, got %q", parsedURL.Scheme)
	}

	client := h.HTTPClient
	if client == nil {
		// Build custom HTTP client if any TLS/proxy options are specified
		if opts.CABundle != "" || opts.ClientCert != "" || opts.ProxyURL != "" {
			tlsOpts := &tlsClientOptions{
				CABundle:   opts.CABundle,
				ClientCert: opts.ClientCert,
				ClientKey:  opts.ClientKey,
				ProxyURL:   opts.ProxyURL,
			}
			tlsClient, err := createTLSClient(tlsOpts)
			if err != nil {
				return fmt.Errorf("failed to configure HTTP client: %w", err)
			}
			client = tlsClient
		} else {
			client = http.DefaultClient
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return err
	}

	// Add Authorization header if specified
	if opts.AuthHeader != "" {
		req.Header.Set("Authorization", opts.AuthHeader)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	tmp := filepath.Clean(opts.Dst + ".tmp")
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	// Cleanup temp file on any error; os.Remove will no-op if file was already renamed.
	defer func() {
		if err := out.Close(); err != nil {
			// Log but don't fail - main operation error is more important
			log.V(1).Info("failed to close temp file during cleanup", "error", err)
		}
		if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
			// Log but don't fail - file may have been renamed successfully
			log.V(1).Info("failed to remove temp file during cleanup", "error", err)
		}
	}()

	hasher := newHasher(opts.Checksum)
	if opts.Checksum != "" && hasher == nil {
		return fmt.Errorf("invalid checksum format")
	}
	writer := io.Writer(out)
	if hasher != nil {
		writer = io.MultiWriter(out, hasher)
	}
	if _, err := io.Copy(writer, resp.Body); err != nil {
		return err
	}
	if hasher != nil {
		if err := hasher.verify(); err != nil {
			return err
		}
	}

	if err := out.Close(); err != nil {
		return err
	}

	return os.Rename(tmp, opts.Dst)
}

// tlsClientOptions contains options for creating a custom HTTP client.
type tlsClientOptions struct {
	CABundle   string
	ClientCert string
	ClientKey  string
	ProxyURL   string
}

// createTLSClient creates an HTTP client configured with custom TLS and proxy settings.
// It clones http.DefaultTransport to preserve default settings (timeouts, keep-alives).
func createTLSClient(opts *tlsClientOptions) (*http.Client, error) {
	// Clone DefaultTransport to preserve default settings like timeouts and keep-alives.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Configure CA bundle for server certificate verification
	if opts.CABundle != "" {
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM([]byte(opts.CABundle)) {
			return nil, fmt.Errorf("failed to parse CA bundle: no valid certificates found")
		}
		transport.TLSClientConfig.RootCAs = caCertPool
	}

	// Configure client certificate for mTLS
	if opts.ClientCert != "" || opts.ClientKey != "" {
		if opts.ClientCert == "" || opts.ClientKey == "" {
			return nil, fmt.Errorf("both client certificate and key must be provided for mTLS")
		}
		cert, err := tls.X509KeyPair([]byte(opts.ClientCert), []byte(opts.ClientKey))
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
	}

	// Configure proxy
	if opts.ProxyURL != "" {
		proxyURL, err := parseProxyURL(opts.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   DefaultDownloadTimeout,
	}, nil
}

// parseProxyURL parses and validates a proxy URL string.
func parseProxyURL(proxyURL string) (*url.URL, error) {
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("proxy URL must use http or https scheme, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("proxy URL must have a host")
	}
	return parsed, nil
}

func (h *Handler) stageBinary(ctx context.Context, archivePath, targetPath string) error {
	// Detect archive type using magic bytes instead of file extension
	// since downloaded files may not have an extension.
	isZip, err := isZipArchive(archivePath)
	if err != nil {
		return fmt.Errorf("failed to detect archive type: %w", err)
	}

	if isZip {
		return extractZip(archivePath, targetPath)
	}

	// Treat as plain binary.
	return copyFile(archivePath, targetPath)
}

// isZipArchive checks if a file is a ZIP archive by reading its magic bytes.
func isZipArchive(path string) (bool, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return false, err
	}
	defer f.Close()

	header := make([]byte, 4)
	n, err := io.ReadFull(f, header)
	if err != nil {
		// File too small to be a zip, treat as binary
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return false, nil
		}
		return false, err
	}
	if n < 4 {
		return false, nil
	}

	return bytes.Equal(header, zipMagic), nil
}

func (h *Handler) updateCurrentSymlink(targetBinary string) error {
	currentLink := h.currentSymlinkPath()
	if err := os.Remove(currentLink); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove current symlink: %w", err)
	}
	linkTarget := targetBinary
	if absRoot, err := filepath.Abs(h.rootPath()); err == nil {
		if absTarget, err := filepath.Abs(targetBinary); err == nil {
			if relTarget, err := filepath.Rel(absRoot, absTarget); err == nil && relTarget != "" && !strings.HasPrefix(relTarget, "..") {
				linkTarget = relTarget
			} else {
				linkTarget = absTarget
			}
		}
	}
	return os.Symlink(linkTarget, currentLink)
}

func (h *Handler) currentSymlinkPath() string {
	return filepath.Join(h.rootPath(), "current")
}

func (h *Handler) versionDir(version string) string {
	// Version is validated by ValidateVersionForPath() before reaching here.
	// safePath provides defense-in-depth against path traversal.
	path, err := safePath(h.rootPath(), "versions", version)
	if err != nil {
		// This should never happen with validated version - indicates a bug
		panic(fmt.Sprintf("versionDir: invalid path for version %q: %v", version, err))
	}
	return path
}

func (h *Handler) versionBinaryPath(version string) string {
	return filepath.Join(h.versionDir(version), "terraform")
}

func (h *Handler) versionArchivePath(version string) string {
	return filepath.Join(h.versionDir(version), "terraform-download")
}

func (h *Handler) rootPath() string {
	if h.RootPath == "" {
		return "/terraform"
	}
	return h.RootPath
}

// safePath constructs a path within root and validates it doesn't escape.
// This prevents path traversal attacks even if version validation is bypassed.
func safePath(root string, subpaths ...string) (string, error) {
	// Clean the root first
	root = filepath.Clean(root)

	// Join and clean the full path
	parts := append([]string{root}, subpaths...)
	full := filepath.Clean(filepath.Join(parts...))

	// Verify the result is within root (has root as prefix)
	// Use filepath.Rel to check - if result starts with "..", it escaped
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes root directory")
	}

	return full, nil
}

func (h *Handler) defaultTerraformURL(version string) string {
	base := strings.TrimSuffix(h.BaseURL, "/")
	if base == "" {
		base = "https://releases.hashicorp.com"
	}
	return fmt.Sprintf("%s/terraform/%s/terraform_%s_%s_%s.zip", base, version, version, runtime.GOOS, runtime.GOARCH)
}

// generateVersionFromURL creates a deterministic, path-safe version identifier
// from a source URL. Used for sourceUrl-only installs where no version is specified.
func generateVersionFromURL(sourceURL string) string {
	h := sha256.Sum256([]byte(sourceURL))
	return "custom-" + hex.EncodeToString(h[:8])
}

type sha256Verifier struct {
	expected []byte
	sum      hash.Hash
}

func newHasher(checksum string) *sha256Verifier {
	if strings.TrimSpace(checksum) == "" {
		return nil
	}

	trimmed := checksum
	if strings.Contains(checksum, ":") {
		parts := strings.SplitN(checksum, ":", 2)
		trimmed = parts[1]
	}
	expected, err := hex.DecodeString(trimmed)
	if err != nil || len(expected) != sha256.Size {
		return nil
	}

	return &sha256Verifier{
		expected: expected,
		sum:      sha256.New(),
	}
}

func (v *sha256Verifier) Write(p []byte) (int, error) {
	return v.sum.Write(p)
}

func (v *sha256Verifier) verify() error {
	if v == nil {
		return nil
	}
	actual := v.sum.Sum(nil)
	if !bytes.Equal(actual, v.expected) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s",
			hex.EncodeToString(v.expected), hex.EncodeToString(actual))
	}
	return nil
}

func extractZip(src, targetPath string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	extracted := false
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if extracted {
			return fmt.Errorf("archive contains multiple files")
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}

		// Use 0o700 for executables - don't trust permissions from downloaded archives
		if err := writeFile(rc, targetPath, 0o700); err != nil {
			_ = rc.Close()
			return err
		}
		if err := rc.Close(); err != nil {
			return err
		}
		extracted = true
	}
	if !extracted {
		return fmt.Errorf("no file found in archive")
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(filepath.Clean(src))
	if err != nil {
		return err
	}
	defer in.Close()

	// Use 0o700 for executable - only owner needs access
	return writeFile(in, dst, 0o700)
}

func writeFile(r io.Reader, dst string, perm os.FileMode) error {
	tmp := filepath.Clean(dst + ".tmp")
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}

	// Limit decompressed size to protect against ZIP bomb attacks
	limitedReader := io.LimitReader(r, MaxDecompressedSize+1)
	n, err := io.Copy(out, limitedReader)
	if err != nil {
		_ = out.Close()
		return err
	}
	if n > MaxDecompressedSize {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("decompressed file exceeds maximum allowed size of %d bytes", MaxDecompressedSize)
	}

	if err := out.Close(); err != nil {
		return err
	}

	if perm != 0 {
		// Mask permissions to standard rwx bits only - prevent setuid/setgid/sticky bits
		// from malicious ZIP archives that could enable privilege escalation
		if err := os.Chmod(tmp, perm&0o777); err != nil {
			return err
		}
	}

	return os.Rename(tmp, dst)
}

func (h *Handler) getOrInitStatus(ctx context.Context) (*Status, error) {
	status, err := h.StatusStore.Get(ctx)
	if err != nil {
		return nil, err
	}
	if status.Versions == nil {
		status.Versions = map[string]VersionStatus{}
	}
	return status, nil
}

func (h *Handler) persistStatus(ctx context.Context, status *Status) error {
	status.LastUpdated = time.Now().UTC()
	if err := h.StatusStore.Put(ctx, status); err != nil {
		ucplog.FromContextOrDiscard(ctx).Error(err, "failed to persist installer status")
		return err
	}
	return nil
}

func (h *Handler) recordFailure(ctx context.Context, status *Status, version string, cause error) error {
	vs := status.Versions[version]
	vs.State = VersionStateFailed
	vs.LastError = cause.Error()
	vs.Health = HealthUnhealthy
	status.Versions[version] = vs
	status.LastError = cause.Error()
	return h.persistStatus(ctx, status)
}

// updateQueueState decrements pending count and sets in-progress operation.
func (h *Handler) updateQueueState(ctx context.Context, inProgress string) {
	updateQueueInfo(ctx, h.StatusStore, func(q *QueueInfo) {
		if q.Pending > 0 {
			q.Pending--
		}
		q.InProgress = &inProgress
	})
}

// clearQueueInProgress clears the in-progress operation.
func (h *Handler) clearQueueInProgress(ctx context.Context) {
	updateQueueInfo(ctx, h.StatusStore, func(q *QueueInfo) {
		q.InProgress = nil
	})
}

func (h *Handler) acquireLock() (*os.File, error) {
	// lockPath uses only trusted h.rootPath() and a constant filename - no user input
	lockPath := filepath.Clean(filepath.Join(h.rootPath(), ".terraform-installer.lock"))
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, ErrInstallerBusy
		}
		return nil, fmt.Errorf("failed to acquire installer lock: %w", err)
	}
	return f, nil
}

func (h *Handler) releaseLock(log logr.Logger, f *os.File) {
	if f == nil {
		return
	}
	lockPath := f.Name()
	_ = f.Close()
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		log.Error(err, "failed to remove installer lock file", "path", lockPath)
	}
}

func (h *Handler) ensureRoot() error {
	return os.MkdirAll(h.rootPath(), 0o750)
}
