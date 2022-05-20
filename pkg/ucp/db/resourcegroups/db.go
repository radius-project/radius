// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package db

import (
	"context"
	"encoding/json"

	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

func GetByID(ctx context.Context, db store.StorageClient, ID resources.ID) (rest.ResourceGroup, error) {
	var rg rest.ResourceGroup
	resp, err := db.Get(ctx, ID)
	if err != nil {
		return rg, err
	}
	if resp != nil {
		err = json.Unmarshal(resp.Data, &rg)
	}
	return rg, err
}

func Save(ctx context.Context, db store.StorageClient, rg rest.ResourceGroup) (rest.ResourceGroup, error) {
	var o store.Object
	var storedResourceGroup rest.ResourceGroup
	//TODO: set the right API version and ETag
	o.Metadata.ContentType = "application/json"
	id := resources.UCPPrefix + rg.ID
	o.Metadata.ID = id
	bytes, err := json.Marshal(rg)
	if err != nil {
		return rest.ResourceGroup{}, err
	}
	o.Data = bytes
	err = db.Save(ctx, &o)
	if err == nil {
		storedResourceGroup = rg
	}
	return storedResourceGroup, err
}

func GetScope(ctx context.Context, db store.StorageClient, query store.Query) (rest.ResourceGroupList, error) {
	listOfResourceGroups := rest.ResourceGroupList{
		Value: []rest.ResourceGroup{},
	}
	resp, err := db.Query(ctx, query)
	if err != nil {
		return listOfResourceGroups, err
	}
	if len(resp) > 0 {
		for _, item := range resp {
			var rg rest.ResourceGroup
			err = json.Unmarshal(item.Data, &rg)
			if err != nil {
				return listOfResourceGroups, err
			}
			listOfResourceGroups.Value = append(listOfResourceGroups.Value, rg)
		}
	}
	return listOfResourceGroups, nil
}

func DeleteByID(ctx context.Context, db store.StorageClient, ID resources.ID) error {
	err := db.Delete(ctx, ID)
	return err
}
