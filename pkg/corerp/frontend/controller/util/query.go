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

package util

import (
	"context"

	"github.com/project-radius/radius/pkg/ucp/store"
)

// # Function Explanation
//
// FindResources searches for resources of a given type with a given filter key and value, and returns the query result.
func FindResources(ctx context.Context, rootScope, resourceType, filterKey, filterValue string, storageClient store.StorageClient) (*store.ObjectQueryResult, error) {
	query := store.Query{
		RootScope:    rootScope,
		ResourceType: resourceType,
		Filters: []store.QueryFilter{
			{
				Field: filterKey,
				Value: filterValue,
			},
		},
	}
	return storageClient.Query(ctx, query)
}
