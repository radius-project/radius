// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

import (
	"context"
	"errors"
	"sync"

	"github.com/Azure/radius/pkg/azure/azresources"
)

// NewInMemoryRadrpDB returns an in-memory implementation of RadrpDB
func NewInMemoryRadrpDB() RadrpDB {
	store := &store{
		operations: map[string]*Operation{},
		mutex:      sync.Mutex{},
	}

	return store
}

type store struct {
	operations map[string]*Operation
	mutex      sync.Mutex
}

var _ RadrpDB = &store{}

func (s *store) GetOperationByID(ctx context.Context, id azresources.ResourceID) (*Operation, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	op := s.operations[id.ID]
	if op == nil {
		return nil, ErrNotFound
	}

	return op, nil
}

func (s *store) PatchOperationByID(ctx context.Context, id azresources.ResourceID, patch *Operation) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, ok := s.operations[id.ID]
	s.operations[id.ID] = patch
	return !ok, nil
}

func (s *store) DeleteOperationByID(ctx context.Context, id azresources.ResourceID) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.operations, id.ID)
	return nil
}

func (s *store) ListV3Applications(ctx context.Context, id azresources.ResourceID) ([]ApplicationResource, error) {
	return nil, errors.New("not implemented")
}

func (s *store) GetV3Application(ctx context.Context, id azresources.ResourceID) (ApplicationResource, error) {
	return ApplicationResource{}, errors.New("not implemented")
}

func (s *store) UpdateV3ApplicationDefinition(ctx context.Context, application ApplicationResource) (bool, error) {
	return false, errors.New("not implemented")
}

func (s *store) DeleteV3Application(ctx context.Context, id azresources.ResourceID) error {
	return errors.New("not implemented")
}

func (s *store) ListAllV3ResourcesByApplication(ctx context.Context, id azresources.ResourceID) ([]RadiusResource, error) {
	return nil, errors.New("not implemented")
}

func (s *store) ListV3Resources(ctx context.Context, id azresources.ResourceID) ([]RadiusResource, error) {
	return nil, errors.New("not implemented")
}

func (s *store) GetV3Resource(ctx context.Context, id azresources.ResourceID) (RadiusResource, error) {
	return RadiusResource{}, errors.New("not implemented")
}

func (s *store) UpdateV3ResourceDefinition(ctx context.Context, id azresources.ResourceID, resource RadiusResource) (bool, error) {
	return false, errors.New("not implemented")
}

func (s *store) UpdateV3ResourceStatus(ctx context.Context, id azresources.ResourceID, resource RadiusResource) error {
	return errors.New("not implemented")
}

func (s *store) DeleteV3Resource(ctx context.Context, id azresources.ResourceID) error {
	return errors.New("not implemented")
}
