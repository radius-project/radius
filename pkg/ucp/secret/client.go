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
	// Returns ErrInvalid in case of invalid input.
	Save(ctx context.Context, name string, value []byte) error

	// Delete deletes secrets of id.
	Delete(ctx context.Context, name string) error

	// Get gets secret name if present else returns an error.
	// Returns ErrNotFound in case of invalid input.
	Get(ctx context.Context, name string) ([]byte, error)
}

// SaveSecret saves a generic secret value using secret client.
func SaveSecret[T any](ctx context.Context, value T, name string, client Client) error {
	secretData, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return client.Save(ctx, name, secretData)
}

// GetSecret gets a generic secret value using secret client.
func GetSecret[T any](ctx context.Context, name string, client Client) (T, error) {
	secretData, err := client.Get(ctx, name)
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

var _ error = (*ErrNotFound)(nil)

type ErrNotFound struct {
}

func (e *ErrNotFound) Error() string {
	return "the resource was not found"
}

func (e *ErrNotFound) Is(target error) bool {
	_, ok := target.(*ErrNotFound)
	return ok
}

var _ error = (*ErrInvalid)(nil)

type ErrInvalid struct {
	Message string
}

func (e *ErrInvalid) Error() string {
	return e.Message
}

func (e *ErrInvalid) Is(target error) bool {
	t, ok := target.(*ErrInvalid)
	if !ok {
		return false
	}

	return (e.Message == t.Message || t.Message == "")
}