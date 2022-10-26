// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secret

import (
	"context"
	"encoding/json"
)

//go:generate mockgen -destination=./mock_client.go -package=secret -self_package github.com/project-radius/radius/pkg/ucp/secret github.com/project-radius/radius/pkg/ucp/secret Client

// Client is an interface to implement secret operations.
type Client interface {
	// Save creates or updates secrets.
	Save(ctx context.Context, value []byte, id string) error
	// Delete deletes secrets of id.
	Delete(ctx context.Context, id string) error
	// Get gets secret name if present else returns an error.
	Get(ctx context.Context, id string) ([]byte, error)
}

// SaveSecret saves a generic secret value using secret client.
func SaveSecret[T any](ctx context.Context, value T, id string, client Client) error {
	secretData, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return client.Save(ctx, secretData, id)
}

// GetSecret gets a generic secret value using secret client.
func GetSecret[T any](ctx context.Context, id string, client Client) (T, error) {
	secretData, err := client.Get(ctx, id)
	var res T
	if err != nil {
		return res, err
	}
	err = json.Unmarshal(secretData, &res)
	if err != nil {
		return res, err
	}
	return res, nil
}
