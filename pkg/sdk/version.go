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

package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/version"
)

// GetVersion reads the release information from the connected Radius API.
func GetVersion(ctx context.Context, connection Connection) (version.VersionInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	endpoint := strings.TrimRight(connection.Endpoint(), "/") + "/version"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		// A url.Error includes the endpoint, which can contain connection credentials.
		if urlErr, ok := err.(*url.Error); ok {
			err = urlErr.Err
		}
		return version.VersionInfo{}, fmt.Errorf("failed to create Radius version request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := connection.Client().Do(req)
	if err != nil {
		// A url.Error includes the endpoint, which can contain connection credentials.
		if urlErr, ok := err.(*url.Error); ok {
			err = urlErr.Err
		}
		return version.VersionInfo{}, fmt.Errorf("failed to read Radius version: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return version.VersionInfo{}, fmt.Errorf("Radius version endpoint returned HTTP %d", resp.StatusCode)
	}

	var info version.VersionInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&info); err != nil {
		return version.VersionInfo{}, fmt.Errorf("invalid Radius version response: %w", err)
	}
	if info.Release == "" {
		return version.VersionInfo{}, fmt.Errorf("Radius version response has no release")
	}
	return info, nil
}
