// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dataprovider

import (
	context "context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"sync"

	store "github.com/project-radius/radius/pkg/store"
)

func NewInMemoryProvider(ctx context.Context, options StorageProviderOptions, resourceType string) (store.StorageClient, error) {
	return &InMemoryStore{options: options, resourceType: resourceType, m: sync.Mutex{}, data: map[string]store.Object{}}, nil
}

type InMemoryStore struct {
	options      StorageProviderOptions
	resourceType string

	m    sync.Mutex
	data map[string]store.Object
}

func (s *InMemoryStore) Query(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
	s.m.Lock()
	defer s.m.Unlock()

	result := store.ObjectQueryResult{}

	return &result, nil
}

func (s *InMemoryStore) Get(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
	s.m.Lock()
	defer s.m.Unlock()

	result, ok := s.data[id]
	if ok {
		return &result, nil
	}

	return nil, &store.ErrNotFound{}
}

func (s *InMemoryStore) Delete(ctx context.Context, id string, options ...store.DeleteOptions) error {
	s.m.Lock()
	defer s.m.Unlock()

	_, ok := s.data[id]
	if ok {
		delete(s.data, id)
		return nil
	}

	return &store.ErrNotFound{}
}

func (s *InMemoryStore) Save(ctx context.Context, obj *store.Object, options ...store.SaveOptions) (*store.Object, error) {
	s.m.Lock()
	defer s.m.Unlock()

	b, err := json.Marshal(obj.Data)
	if err != nil {
		return nil, err
	}

	hash := sha1.Sum(b)
	etag := fmt.Sprintf("\"%d-%x\"", int(len(hash)), hash)
	obj.ETag = etag

	s.data[obj.ID] = *obj

	return obj, nil
}
