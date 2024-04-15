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

package sdk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// ErrRadiusNotInstalled is the error reported when Radius is not installed for a connection.
type ErrRadiusNotInstalled struct {
}

// Is checks if the given error is an instance of ErrRadiusNotInstalled.
func (*ErrRadiusNotInstalled) Is(other error) bool {
	_, ok := other.(*ErrRadiusNotInstalled)
	return ok
}

// ErrRadiusNotInstalled returns an error message when a Radius installation cannot be found.
func (*ErrRadiusNotInstalled) Error() string {
	return "a Radius installation could not be found. Use 'rad install kubernetes' to install"
}

// TestConnection tests the provided connection to determine if the Radius API is responding. This
// will return ErrRadiusNotInstalled when it can be determined that Radius is not installed, and
// a generic error for other failure conditions.
//
// Creating a new connection with the various New functions in this package does not call TestConnection
// automatically. This allows a connection to be created before Radius has been installed.
func TestConnection(ctx context.Context, connection Connection) error {
	req, err := createHealthCheckRequest(ctx, connection.Endpoint())
	if err != nil {
		return err
	}

	resp, err := connection.Client().Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound {
		return &ErrRadiusNotInstalled{}
	} else if resp.StatusCode >= 400 {
		return reportErrorFromResponse(resp)
	}

	return nil
}

func createHealthCheckRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	return req, nil
}

func reportErrorFromResponse(resp *http.Response) error {
	message := &strings.Builder{}
	_, _ = message.WriteString("An unknown error was returned while testing Radius API status:\n")
	_, _ = message.WriteString(fmt.Sprintf("Status Code: %d\n", resp.StatusCode))

	_, _ = message.WriteString("Response Headers:\n")
	keys := []string{}
	for key := range resp.Header {
		keys = append(keys, key)
	}

	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range resp.Header[key] {
			_, _ = message.WriteString(fmt.Sprintf("  %s: %s\n", key, value))
		}
	}

	if resp.Body == nil {
		_, _ = message.WriteString("Response Body: (empty)\n")
	} else {
		defer resp.Body.Close()

		_, _ = message.WriteString("Response Body:\n")
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			_, _ = message.WriteString(fmt.Sprintf("  Error reading response body: %s\n", err))
			return errors.New(message.String())
		}

		_, _ = message.Write(b)
		_, _ = message.WriteString("\n")
	}

	return errors.New(message.String())
}
