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

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/terraform/installer"
)

// VersionInfo represents a Terraform version for display purposes.
type VersionInfo struct {
	Version     string    `json:"version"`
	State       string    `json:"state"`
	Health      string    `json:"health"`
	InstalledAt time.Time `json:"installedAt"`
	IsCurrent   bool      `json:"isCurrent"`
}

// Client provides methods for interacting with the Terraform installer API.
type Client struct {
	connection sdk.Connection
}

// NewClient creates a new installer client using the provided SDK connection.
func NewClient(connection sdk.Connection) *Client {
	return &Client{connection: connection}
}

// baseURL returns the installer API base URL.
func (c *Client) baseURL() string {
	endpoint := strings.TrimSuffix(c.connection.Endpoint(), "/")
	return endpoint + "/installer/terraform"
}

// Install sends an install request to the installer API.
func (c *Client) Install(ctx context.Context, req installer.InstallRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal install request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+"/install", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create install request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.connection.Client().Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send install request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.parseErrorResponse(resp)
	}

	return nil
}

// Uninstall sends an uninstall request to the installer API.
func (c *Client) Uninstall(ctx context.Context, req installer.UninstallRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal uninstall request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+"/uninstall", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create uninstall request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.connection.Client().Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send uninstall request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.parseErrorResponse(resp)
	}

	return nil
}

// Status retrieves the current installer status.
func (c *Client) Status(ctx context.Context) (*installer.StatusResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL()+"/status", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create status request: %w", err)
	}

	resp, err := c.connection.Client().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send status request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.parseErrorResponse(resp)
	}

	var status installer.StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode status response: %w", err)
	}

	return &status, nil
}

// parseErrorResponse reads the error response body and returns an appropriate error.
func (c *Client) parseErrorResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return clierrors.Message("Request failed with status %d", resp.StatusCode)
	}

	bodyStr := strings.TrimSpace(string(body))
	if bodyStr == "" {
		return clierrors.Message("Request failed with status %d", resp.StatusCode)
	}

	return clierrors.Message("Request failed with status %d: %s", resp.StatusCode, bodyStr)
}

// VersionsToList converts a versions map to a sorted slice for display.
// The current version is marked with IsCurrent=true.
func VersionsToList(versions map[string]installer.VersionStatus, currentVersion string) []VersionInfo {
	if len(versions) == 0 {
		return nil
	}

	result := make([]VersionInfo, 0, len(versions))
	for _, vs := range versions {
		result = append(result, VersionInfo{
			Version:     vs.Version,
			State:       string(vs.State),
			Health:      string(vs.Health),
			InstalledAt: vs.InstalledAt,
			IsCurrent:   vs.Version == currentVersion,
		})
	}

	// Sort by version descending (newest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Version > result[j].Version
	})

	return result
}
