// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package util

import (
	"context"

	"github.com/project-radius/radius/pkg/ucp/store"
)

// FindResources queries all resources matched with resource type and property value.
func FindResources(ctx context.Context, rootScope, resourceType, filterKey, filterValue string, storageClient store.StorageClient) (*store.ObjectQueryResult, error) {
	namespaceQuery := store.Query{
		RootScope:    rootScope,
		ResourceType: resourceType,
		Filters: []store.QueryFilter{
			{
				Field: filterKey,
				Value: filterValue,
			},
		},
	}
	return storageClient.Query(ctx, namespaceQuery)
}
