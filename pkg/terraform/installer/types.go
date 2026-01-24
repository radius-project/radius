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
	"context"
	"time"
)

// Operation enumerates installer operations.
type Operation string

const (
	// OperationInstall enqueues a Terraform install.
	OperationInstall Operation = "install"
	// OperationUninstall enqueues a Terraform uninstall.
	OperationUninstall Operation = "uninstall"
)

// VersionState enumerates installer states for a version.
type VersionState string

const (
	VersionStateInstalling   VersionState = "Installing"
	VersionStateSucceeded    VersionState = "Succeeded"
	VersionStateFailed       VersionState = "Failed"
	VersionStateUninstalling VersionState = "Uninstalling"
	VersionStateUninstalled  VersionState = "Uninstalled"
)

// HealthStatus enumerates health of an installed version.
type HealthStatus string

const (
	HealthUnknown   HealthStatus = "Unknown"
	HealthHealthy   HealthStatus = "Healthy"
	HealthUnhealthy HealthStatus = "Unhealthy"
)

// InstallRequest describes an install submission.
type InstallRequest struct {
	// Version requested for install (for example 1.6.4).
	Version string `json:"version"`
	// SourceURL is an optional direct archive URL to download Terraform from.
	SourceURL string `json:"sourceUrl"`
	// Checksum is an optional checksum string (for example sha256:<hash>).
	Checksum string `json:"checksum"`
	// CABundle is an optional PEM-encoded CA certificate bundle for TLS verification.
	// Used when downloading from servers with self-signed or private CA certificates.
	CABundle string `json:"caBundle,omitempty"`
	// AuthHeader is an optional HTTP Authorization header value (e.g., "Bearer <token>" or "Basic <base64>").
	// Used when downloading from servers that require authentication.
	AuthHeader string `json:"authHeader,omitempty"`
	// ClientCert is an optional PEM-encoded client certificate for mTLS authentication.
	// Must be used together with ClientKey.
	ClientCert string `json:"clientCert,omitempty"`
	// ClientKey is an optional PEM-encoded client private key for mTLS authentication.
	// Must be used together with ClientCert.
	ClientKey string `json:"clientKey,omitempty"`
	// ProxyURL is an optional HTTP/HTTPS proxy URL (e.g., "http://proxy.corp.com:8080").
	// Used when downloading through a corporate proxy.
	ProxyURL string `json:"proxyUrl,omitempty"`
}

// UninstallRequest describes an uninstall submission.
type UninstallRequest struct {
	// Version to uninstall.
	Version string `json:"version"`
	// Purge removes the version metadata from the database after uninstalling.
	// When false (default), the version entry remains with state "Uninstalled" for audit purposes.
	Purge bool `json:"purge,omitempty"`
}

// Status represents installer status metadata.
type Status struct {
	// Current is the active Terraform version.
	Current string `json:"current,omitempty"`
	// Previous is the prior Terraform version (used for rollback).
	Previous string `json:"previous,omitempty"`
	// Versions captures per-version metadata.
	Versions map[string]VersionStatus `json:"versions,omitempty"`
	// LastError captures the last error message from installer failures.
	LastError string `json:"lastError,omitempty"`
	// LastUpdated records the last time status was updated.
	LastUpdated time.Time `json:"lastUpdated,omitempty"`
	// Queue tracks pending and in-progress installer operations.
	Queue *QueueInfo `json:"queue,omitempty"`
}

// VersionStatus captures metadata for a specific Terraform version.
type VersionStatus struct {
	// Version is the Terraform version string.
	Version string `json:"version,omitempty"`
	// SourceURL used to download this version.
	SourceURL string `json:"sourceUrl,omitempty"`
	// Checksum used to validate the download.
	Checksum string `json:"checksum,omitempty"`
	// State represents the lifecycle state (for example Pending, Succeeded, Failed).
	State VersionState `json:"state,omitempty"`
	// Health captures health diagnostics for this version.
	Health HealthStatus `json:"health,omitempty"`
	// InstalledAt is the timestamp when the version was installed.
	InstalledAt time.Time `json:"installedAt,omitempty"`
	// LastError contains the last error for this version, if any.
	LastError string `json:"lastError,omitempty"`
}

// ExecutionChecker checks for active Terraform executions.
// This is used to prevent uninstalling a Terraform version while recipes are running.
//
// NOTE: This interface should be implemented as necessary when integrating with the
// recipes system. The implementation should query the async operation store for
// in-progress recipe deployments that use the Terraform engine. If no implementation
// is provided to the Handler, the safety check is skipped.
type ExecutionChecker interface {
	// HasActiveExecutions returns true if any recipe executions using Terraform are in progress.
	HasActiveExecutions(ctx context.Context) (bool, error)
}

// ResponseState enumerates API response states (per design doc).
type ResponseState string

const (
	ResponseStateNotInstalled ResponseState = "not-installed"
	ResponseStateInstalling   ResponseState = "installing"
	ResponseStateReady        ResponseState = "ready"
	ResponseStateUninstalling ResponseState = "uninstalling"
	ResponseStateFailed       ResponseState = "failed"
)

// StatusResponse is the HTTP API response format (matches design doc).
type StatusResponse struct {
	// CurrentVersion is the active Terraform version.
	CurrentVersion string `json:"currentVersion,omitempty"`
	// State is the overall installer state.
	State ResponseState `json:"state,omitempty"`
	// BinaryPath is the path to the active Terraform binary.
	BinaryPath string `json:"binaryPath,omitempty"`
	// InstalledAt is the timestamp when the current version was installed.
	InstalledAt *time.Time `json:"installedAt,omitempty"`
	// Source contains the URL and checksum used for the current version.
	Source *SourceInfo `json:"source,omitempty"`
	// Queue contains queue status information.
	Queue *QueueInfo `json:"queue,omitempty"`
	// History contains recent operation history.
	History []HistoryEntry `json:"history,omitempty"`
	// Versions contains per-version metadata (for detailed status queries).
	Versions map[string]VersionStatus `json:"versions,omitempty"`
	// LastError captures the last error message from installer failures.
	LastError string `json:"lastError,omitempty"`
	// LastUpdated records the last time status was updated.
	LastUpdated time.Time `json:"lastUpdated,omitempty"`
}

// SourceInfo contains download source information.
type SourceInfo struct {
	URL      string `json:"url,omitempty"`
	Checksum string `json:"checksum,omitempty"`
}

// QueueInfo contains queue status information.
type QueueInfo struct {
	Pending    int     `json:"pending"`
	InProgress *string `json:"inProgress,omitempty"`
}

// HistoryEntry represents a single operation in the history.
type HistoryEntry struct {
	Operation string    `json:"operation"`
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

// ToResponse converts internal Status to API StatusResponse format.
func (s *Status) ToResponse(rootPath string) StatusResponse {
	// Use tracked queue info if available, otherwise default to empty
	queueInfo := s.Queue
	if queueInfo == nil {
		queueInfo = &QueueInfo{Pending: 0}
	}

	resp := StatusResponse{
		CurrentVersion: s.Current,
		Versions:       s.Versions,
		LastError:      s.LastError,
		LastUpdated:    s.LastUpdated,
		Queue:          queueInfo,
	}

	// Determine overall state based on current version status
	if s.Current == "" {
		resp.State = ResponseStateNotInstalled
	} else if vs, ok := s.Versions[s.Current]; ok {
		resp.State = mapVersionStateToResponseState(vs.State)
		if !vs.InstalledAt.IsZero() {
			resp.InstalledAt = &vs.InstalledAt
		}
		if vs.SourceURL != "" || vs.Checksum != "" {
			resp.Source = &SourceInfo{
				URL:      vs.SourceURL,
				Checksum: vs.Checksum,
			}
		}
	} else {
		resp.State = ResponseStateNotInstalled
	}

	// Build binary path if we have a current version
	if s.Current != "" && rootPath != "" {
		resp.BinaryPath = rootPath + "/versions/" + s.Current + "/terraform"
	}

	return resp
}

// mapVersionStateToResponseState maps internal VersionState to API ResponseState.
func mapVersionStateToResponseState(vs VersionState) ResponseState {
	switch vs {
	case VersionStateInstalling:
		return ResponseStateInstalling
	case VersionStateSucceeded:
		return ResponseStateReady
	case VersionStateFailed:
		return ResponseStateFailed
	case VersionStateUninstalling:
		return ResponseStateUninstalling
	case VersionStateUninstalled:
		return ResponseStateNotInstalled
	default:
		return ResponseStateNotInstalled
	}
}
