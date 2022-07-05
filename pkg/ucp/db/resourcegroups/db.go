// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package db

import (
	"context"

	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

func GetByID(ctx context.Context, db store.StorageClient, ID resources.ID) (rest.ResourceGroup, error) {
	var rg rest.ResourceGroup
	resp, err := db.Get(ctx, ID.String())
	if err != nil {
		return rg, err
	}
	if resp != nil {
		err = resp.As(&rg)
	}
	return rg, err
}

func Save(ctx context.Context, db store.StorageClient, rg rest.ResourceGroup) (rest.ResourceGroup, error) {
	var o store.Object
	var storedResourceGroup rest.ResourceGroup
	//TODO: set the right API version and ETag
	o.Metadata.ContentType = "application/json"
	o.Metadata.ID = rg.ID
	o.Data = &rg
	err := db.Save(ctx, &o)
	if err == nil {
		storedResourceGroup = rg
	}
	return storedResourceGroup, err
}

func GetScope(ctx context.Context, db store.StorageClient, query store.Query) (rest.ResourceGroupList, error) {
	result, err := db.Query(ctx, query)
	if err != nil {
		return rest.ResourceGroupList{}, err
	}

	listOfResourceGroups := rest.ResourceGroupList{}
	if result != nil && len(result.Items) > 0 {
		for _, item := range result.Items {
			var rg rest.ResourceGroup
			err = item.As(&rg)
			if err != nil {
				return listOfResourceGroups, err
			}
			listOfResourceGroups.Value = append(listOfResourceGroups.Value, rg)
		}
	}
	return listOfResourceGroups, nil
}

func GetScopeAllResources(ctx context.Context, db store.StorageClient, query store.Query) (rest.ResourceList, error) {
	result, err := db.Query(ctx, query)
	if err != nil {
		return rest.ResourceList{}, err
	}

	if result == nil || len(result.Items) == 0 {
		return rest.ResourceList{}, nil
	}

	listOfResources := rest.ResourceList{}
	for _, item := range result.Items {
		var resource rest.Resource
		err = item.As(&resource)
		if err != nil {
			return listOfResources, err
		}
		listOfResources.Value = append(listOfResources.Value, resource)
	}
	return listOfResources, nil
}

func DeleteByID(ctx context.Context, db store.StorageClient, ID resources.ID) error {
	err := db.Delete(ctx, ID.String())
	return err
}
