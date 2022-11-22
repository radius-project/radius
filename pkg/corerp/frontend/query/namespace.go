// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package query

import (
	"context"

	"github.com/project-radius/radius/pkg/ucp/store"
)

func FindNamespaceResources(ctx context.Context, rootScope, resourceType, filterKey, filterValue string, storageClient store.StorageClient) (*store.ObjectQueryResult, error) {
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
