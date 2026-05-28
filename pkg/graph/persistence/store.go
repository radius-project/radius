// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// Package persistence defines the Store abstraction used to save and load
// serialized graphs. Concrete backends (git, graphdb, ...) live in
// sub-packages.
package persistence

import (
	"context"
	"errors"

	"github.com/radius-project/radius/pkg/graph/serialize"
)

// ErrNotFound is returned by Store.Load when no payload exists for the key.
var ErrNotFound = errors.New("persistence: not found")

// Key identifies a persisted graph payload within a Store.
//
// The meaning of the fields is backend-specific:
//   - For the git backend, Namespace maps to a branch prefix and Name to the
//     file path inside the branch; Version is the commit message/tag.
//   - For a future graph DB backend, these fields map to database/collection
//     identifiers.
type Key struct {
	// Namespace groups related payloads (e.g. a branch prefix or DB collection).
	Namespace string

	// Name identifies the payload within the namespace.
	Name string

	// Version is an optional revision discriminator. When empty, implementations
	// store the payload at a single canonical location for (Namespace, Name).
	Version string
}

// SaveOptions contains optional metadata applied during Save.
type SaveOptions struct {
	// Message is a human-readable description of the change (e.g. git commit
	// message).
	Message string

	// Labels are free-form key/value pairs attached to the saved payload.
	Labels map[string]string
}

// Store persists serialized graph payloads.
//
// Implementations must be safe for concurrent use by multiple goroutines.
type Store interface {
	// Save persists payload under key. Implementations should be idempotent
	// for identical (key, payload) pairs.
	Save(ctx context.Context, key Key, payload *serialize.Payload, opts SaveOptions) error

	// Load returns the payload previously stored under key, or ErrNotFound.
	Load(ctx context.Context, key Key) (*serialize.Payload, error)

	// List returns keys whose Namespace matches the supplied value. An empty
	// namespace lists all keys.
	List(ctx context.Context, namespace string) ([]Key, error)

	// Delete removes the payload stored under key. Deleting a missing key
	// must return ErrNotFound.
	Delete(ctx context.Context, key Key) error
}
