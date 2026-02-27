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
	"errors"

	"github.com/radius-project/radius/pkg/components/database"
)

// StatusStore persists installer status metadata.
type StatusStore interface {
	// Get returns the current installer status.
	Get(ctx context.Context) (*Status, error)
	// Put persists the installer status.
	Put(ctx context.Context, status *Status) error
}

// StatusStoreImpl persists status using the database client.
type StatusStoreImpl struct {
	client database.Client
	// StorageKey allows namespacing installer status.
	StorageKey string
}

// NewStatusStore creates a new StatusStoreImpl.
func NewStatusStore(client database.Client, storageKey string) *StatusStoreImpl {
	return &StatusStoreImpl{
		client:     client,
		StorageKey: storageKey,
	}
}

// Get retrieves installer status from the status manager.
func (s *StatusStoreImpl) Get(ctx context.Context) (*Status, error) {
	result := &Status{}
	obj, err := s.client.Get(ctx, s.StorageKey)
	if err != nil {
		var notFound *database.ErrNotFound
		if errors.As(err, &notFound) {
			return &Status{
				Versions: map[string]VersionStatus{},
			}, nil
		}
		return nil, err
	}

	if err := obj.As(result); err != nil {
		return nil, err
	}

	return result, nil
}

// Put writes installer status through the status manager.
func (s *StatusStoreImpl) Put(ctx context.Context, status *Status) error {
	obj := &database.Object{
		Metadata: database.Metadata{
			ID: s.StorageKey,
		},
		Data: status,
	}

	return s.client.Save(ctx, obj)
}
