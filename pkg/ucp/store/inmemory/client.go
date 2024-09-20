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

package inmemory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/pkg/ucp/store/storeutil"
	"github.com/radius-project/radius/pkg/ucp/util/etag"
	"golang.org/x/exp/maps"
)

var _ store.StorageClient = (*Client)(nil)

// Client is an in-memory implementation of store.StorageClient.
type Client struct {
	// mutex is used to synchronize access to the resources map.
	mutex sync.Mutex

	// resources is a map of resource IDs to their corresponding entries.
	//
	// The Get/Save/Delete methods will use the resource ID directly since they only access
	// a single entry at a time.
	//
	// The Query method will iterate over all entries in the map to find the matching ones.
	resources map[string]entry
}

// entry stores the commonly-used fields (extracted from the resource ID) for comparison in queries.
// This is provided for ease of debugging.
//
// We use the existing normalization logic to simplify comparisons:
//
// - Convert to lowercase
// - Add leading/trailing slashes.
//
// Here's an example:
//
//	resource ID: "/planes/radius/local/resourceGroups/my-rg/providers/Applications.Test/testType1/my-resource/testType2/my-child-resource"
//
// The entry would be:
//
//	rootScope: "/planes/radius/local/resourcegroups/my-rg/"
//	resourceType: "/applications.test/testtype1/testtype2/"
//	routingScope: "/applications.test/testtype1/my-resource/testtype2/my-child-resource/"
//
// All fields are compared case-insensitively.
type entry struct {
	// obj stores the object data.
	obj store.Object

	// rootScope is the root scope of the resource ID.
	rootScope string

	// resourceType is the resource type of the resource ID.
	resourceType string

	// routingScope is the routing scope of the resource ID.
	routingScope string
}

// NewClient creates a new in-memory store client.
func NewClient() *Client {
	return &Client{
		mutex:     sync.Mutex{},
		resources: map[string]entry{},
	}
}

// Get implements store.StorageClient.
func (c *Client) Get(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
	if ctx == nil {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	parsed, err := resources.Parse(id)
	if err != nil {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'id' must be a valid resource id"}
	}
	if parsed.IsEmpty() {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'id' must not be empty"}
	}
	if parsed.IsResourceCollection() || parsed.IsScopeCollection() {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'id' must refer to a named resource, not a collection"}
	}

	normalized, err := storeutil.NormalizeResourceID(parsed)
	if err != nil {
		return nil, err
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	entry, ok := c.resources[strings.ToLower(normalized.String())]
	if !ok {
		return nil, &store.ErrNotFound{ID: id}
	}

	// Make a defensive copy so users can't modify the data in the store.
	copy, err := entry.obj.DeepCopy()
	if err != nil {
		return nil, err
	}

	return copy, nil
}

// Delete implements store.StorageClient.
func (c *Client) Delete(ctx context.Context, id string, options ...store.DeleteOptions) error {
	if ctx == nil {
		return &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	parsed, err := resources.Parse(id)
	if err != nil {
		return &store.ErrInvalid{Message: "invalid argument. 'id' must be a valid resource id"}
	}
	if parsed.IsEmpty() {
		return &store.ErrInvalid{Message: "invalid argument. 'id' must not be empty"}
	}
	if parsed.IsResourceCollection() || parsed.IsScopeCollection() {
		return &store.ErrInvalid{Message: "invalid argument. 'id' must refer to a named resource, not a collection"}
	}

	normalized, err := storeutil.NormalizeResourceID(parsed)
	if err != nil {
		return err
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	config := store.NewDeleteConfig(options...)

	entry, ok := c.resources[strings.ToLower(normalized.String())]
	if !ok && config.ETag != "" {
		return &store.ErrConcurrency{}
	} else if !ok {
		return &store.ErrNotFound{ID: id}
	} else if config.ETag != "" && config.ETag != entry.obj.ETag {
		return &store.ErrConcurrency{}
	}

	delete(c.resources, strings.ToLower(normalized.String()))

	return nil
}

// Query implements store.StorageClient.
func (c *Client) Query(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
	if ctx == nil {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}

	err := query.Validate()
	if err != nil {
		return nil, &store.ErrInvalid{Message: fmt.Sprintf("invalid argument. Query is invalid: %s", err.Error())}
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	result := &store.ObjectQueryResult{}
	for _, entry := range c.resources {
		// Check root scope.
		if query.ScopeRecursive && !strings.HasPrefix(entry.rootScope, storeutil.NormalizePart(query.RootScope)) {
			continue
		} else if !query.ScopeRecursive && entry.rootScope != storeutil.NormalizePart(query.RootScope) {
			continue
		}

		// Check resource type.
		resourceType, err := storeutil.NormalizeResourceType(query.ResourceType)
		if err != nil {
			return nil, err
		}
		if entry.resourceType != storeutil.NormalizePart(resourceType) {
			continue
		}

		// Check routing scope prefix (optional).
		if query.RoutingScopePrefix != "" && !strings.HasPrefix(entry.routingScope, storeutil.NormalizePart(query.RoutingScopePrefix)) {
			continue
		}

		// Check filters (optional).
		match, err := entry.obj.MatchesFilters(query.Filters)
		if err != nil {
			return nil, err
		}
		if !match {
			continue
		}

		// Make a defensive copy so users can't modify the data in the store.
		copy, err := entry.obj.DeepCopy()
		if err != nil {
			return nil, err
		}

		result.Items = append(result.Items, *copy)
	}

	return result, nil
}

// Save implements store.StorageClient.
func (c *Client) Save(ctx context.Context, obj *store.Object, options ...store.SaveOptions) error {
	if ctx == nil {
		return &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	if obj == nil {
		return &store.ErrInvalid{Message: "invalid argument. 'obj' is required"}
	}

	parsed, err := resources.Parse(obj.ID)
	if err != nil {
		return &store.ErrInvalid{Message: "invalid argument. 'obj.ID' must be a valid resource id"}
	}

	normalized, err := storeutil.NormalizeResourceID(parsed)
	if err != nil {
		return err
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	config := store.NewSaveConfig(options...)

	entry, ok := c.resources[strings.ToLower(normalized.String())]
	if !ok && config.ETag != "" {
		return &store.ErrConcurrency{}
	} else if ok && config.ETag != "" && config.ETag != entry.obj.ETag {
		return &store.ErrConcurrency{}
	} else if !ok {
		// New entry, initialize it.
		entry.rootScope = storeutil.NormalizePart(normalized.RootScope())
		entry.resourceType = storeutil.NormalizePart(normalized.Type())
		entry.routingScope = storeutil.NormalizePart(normalized.RoutingScope())
	}

	raw, err := json.Marshal(obj.Data)
	if err != nil {
		return err
	}

	// Updated the ETag before copying. Callers are allowed to read the ETag after calling save.
	obj.ETag = etag.New(raw)

	// Make a defensive copy so users can't modify the data in the store.
	copy, err := obj.DeepCopy()
	if err != nil {
		return err
	}

	entry.obj = *copy

	c.resources[strings.ToLower(normalized.String())] = entry

	return nil
}

// Clear can be used to clear all stored data.
func (c *Client) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	maps.Clear(c.resources)
}
