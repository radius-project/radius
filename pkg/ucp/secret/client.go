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

package secret

import (
	"context"
	"encoding/json"
)

//go:generate mockgen -destination=./mock_client.go -package=secret -self_package github.com/project-radius/radius/pkg/ucp/secret github.com/project-radius/radius/pkg/ucp/secret Client

// Client is an interface to implement secret operations.
type Client interface {
	// Save creates or updates secret.
	// Returns ErrInvalid in case of invalid input.
	Save(ctx context.Context, name string, value []byte) error

	// Delete deletes the secret with the given name.
	Delete(ctx context.Context, name string) error

	// Get gets secret name if present else returns an error.
	// Returns ErrNotFound in case of invalid input.
	Get(ctx context.Context, name string) ([]byte, error)
}

// SaveSecret saves a secret value using secret client, marshalling it to JSON first.
func SaveSecret[T any](ctx context.Context, client Client, name string, value T) error {
	secretData, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return client.Save(ctx, name, secretData)
}

// GetSecret retrieves a secret using secret client and returns it as a generic type, returning an error if the retrieval or
// unmarshalling fails.
func GetSecret[T any](ctx context.Context, client Client, name string) (T, error) {
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

// ErrNotFound represents error when resource is missing.
type ErrNotFound struct {
}

// Error returns the error message.
func (e *ErrNotFound) Error() string {
	return "the resource was not found"
}

// Is checks if the error is of type ErrNotFound.
func (e *ErrNotFound) Is(target error) bool {
	_, ok := target.(*ErrNotFound)
	return ok
}

var _ error = (*ErrInvalid)(nil)

// ErrInvalid represents error when resource inputs are invalid.
type ErrInvalid struct {
	Message string
}

// Error returns a string representation of the error.
func (e *ErrInvalid) Error() string {
	return e.Message
}

// Is checks if the target error is of type ErrInvalid and if the message of the target error is equal to the
// message of the current error or if the message of the target error is empty.
func (e *ErrInvalid) Is(target error) bool {
	t, ok := target.(*ErrInvalid)
	if !ok {
		return false
	}

	return (e.Message == t.Message || t.Message == "")
}
