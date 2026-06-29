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

// Package persistence defines the Store abstraction used to save and load
// ApplicationGraphResponse artifacts. Concrete backends (git, graphdb, ...)
// live in sub-packages.
package persistence

import (
	"context"
	"errors"

	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
)

// ErrNotFound is returned by Store.Load when no graph exists for the key.
var ErrNotFound = errors.New("persistence: not found")

// Key identifies a persisted graph within a Store.
//
// The meaning of the fields is backend-specific:
//   - For the git backend, Namespace maps to a branch prefix and Name to the
//     file path inside the branch.
//   - For a future graph DB backend, these fields map to database/collection
//     identifiers.
type Key struct {
	// Namespace groups related graphs (e.g. a branch prefix or DB collection).
	Namespace string

	// Name identifies the graph within the namespace.
	Name string
}

// SaveOptions contains optional metadata applied during Save.
type SaveOptions struct {
	// Message is a human-readable description of the change (e.g. git commit
	// message).
	Message string

	// Labels are free-form key/value pairs attached to the saved graph.
	Labels map[string]string
}

// Store persists ApplicationGraphResponse artifacts.
//
// Implementations must be safe for concurrent use by multiple goroutines.
//
//go:generate go tool mockgen -typed -destination=./mock_store.go -package=persistence -self_package github.com/radius-project/radius/pkg/graph/persistence github.com/radius-project/radius/pkg/graph/persistence Store
type Store interface {
	// Save persists graph under key. Implementations should be idempotent
	// for identical (key, graph) pairs.
	Save(ctx context.Context, key Key, graph *corerpv20250801preview.ApplicationGraphResponse, opts SaveOptions) error

	// Load returns the graph previously stored under key, or ErrNotFound.
	Load(ctx context.Context, key Key) (*corerpv20250801preview.ApplicationGraphResponse, error)

	// List returns keys whose Namespace matches the supplied value. An empty
	// namespace lists all keys.
	List(ctx context.Context, namespace string) ([]Key, error)

	// Delete removes the graph stored under key. Deleting a missing key
	// must return ErrNotFound.
	Delete(ctx context.Context, key Key) error
}
