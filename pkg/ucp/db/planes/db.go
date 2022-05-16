// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"context"
	"encoding/json"

	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

// Used to get all the matching "Scopes", such as planes, planes of a specific type , resourceGroups ...
func GetScope(ctx context.Context, db store.StorageClient, query store.Query) (rest.PlaneList, error) {
	listOfPlanes := rest.PlaneList{
		Value: []rest.Plane{},
	}
	resp, err := db.Query(ctx, query)
	if err != nil {
		return listOfPlanes, err
	}
	if len(resp) > 0 {
		for _, item := range resp {
			var plane rest.Plane
			err = json.Unmarshal(item.Data, &plane)
			if err != nil {
				return listOfPlanes, err
			}
			listOfPlanes.Value = append(listOfPlanes.Value, plane)
		}
	}
	return listOfPlanes, nil
}

func GetByID(ctx context.Context, db store.StorageClient, ID resources.ID) (rest.Plane, error) {
	var plane rest.Plane
	resp, err := db.Get(ctx, ID)
	if err != nil {
		return plane, err
	}
	if resp != nil {
		err = json.Unmarshal(resp.Data, &plane)
	}
	return plane, err
}

func Save(ctx context.Context, db store.StorageClient, plane rest.Plane) (rest.Plane, error) {
	var o store.Object
	var storedPlane rest.Plane
	//TODO: set the right API version and ETag
	o.Metadata.ContentType = "application/json"
	id := resources.UCPPrefix + plane.ID
	o.Metadata.ID = id
	bytes, err := json.Marshal(plane)
	if err != nil {
		return rest.Plane{}, err
	}
	o.Data = bytes
	err = db.Save(ctx, &o)
	if err == nil {
		storedPlane = plane
	}
	return storedPlane, err
}

func DeleteByID(ctx context.Context, db store.StorageClient, ID resources.ID) error {
	err := db.Delete(ctx, ID)
	return err
}
