// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sdk

import (
	"context"
	"fmt"
	"net/http"
)

// ErrRadiusNotInstalled is the error reported when Radius is not installed for a connection.
type ErrRadiusNotInstalled struct {
}

// Is determines whether the provided error is an ErrRadiusNotInstalled.
func (*ErrRadiusNotInstalled) Is(other error) bool {
	_, ok := other.(*ErrRadiusNotInstalled)
	return ok
}

// Error is the Error() implementation.
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
		return fmt.Errorf("an unknown error occurred, status code was %d", resp.StatusCode)
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
