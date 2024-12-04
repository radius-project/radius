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

package planes

import (
	"context"
	"fmt"
	http "net/http"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/middleware"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*ListPlanesByType[*datamodel.GenericPlane, datamodel.GenericPlane])(nil)

// ListPlanesByType is the controller implementation to get the list of UCP planes.
type ListPlanesByType[P interface {
	*T
	v1.ResourceDataModel
}, T any] struct {
	armrpc_controller.Operation[P, T]
}

// ListPlanesByType takes in a request object and returns a list of planes of a given type from the storage client. If
// an error occurs, it returns an error.
func (e *ListPlanesByType[P, T]) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	path := middleware.GetRelativePath(e.Options().PathBase, req.URL.Path)
	// The path is /planes/{planeShortType}
	planeShortType := strings.Split(path, resources.SegmentSeparator)[2]

	// Map that onto the known plane fully-qualified types, so we can do the database
	// lookup.
	knownPlaneTypes := map[string]string{
		"aws":    "System.AWS/planes",
		"azure":  "System.Azure/planes",
		"radius": "System.Radius/planes",
	}

	planeType, ok := knownPlaneTypes[planeShortType]
	if !ok {
		return armrpc_rest.NewBadRequestResponse(fmt.Sprintf("Unknown plane type %s", planeShortType)), nil
	}

	query := store.Query{
		RootScope:    resources.SegmentSeparator + resources.PlanesSegment,
		IsScopeQuery: true,
		ResourceType: planeType,
	}
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Listing planes in scope %s/%s", query.RootScope, planeType))

	client, err := e.DataProvider().GetStorageClient(ctx, planeType)
	if err != nil {
		return nil, err
	}

	result, err := client.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	listOfPlanes, err := e.createResponse(ctx, result)
	if err != nil {
		return nil, err
	}

	return armrpc_rest.NewOKResponse(listOfPlanes), nil
}

func (p *ListPlanesByType[P, T]) createResponse(ctx context.Context, result *store.ObjectQueryResult) (*v1.PaginatedList, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	items := v1.PaginatedList{}

	for _, item := range result.Items {
		var plane T
		err := item.As(&plane)
		if err != nil {
			return nil, err
		}

		versioned, err := p.ResponseConverter()(&plane, serviceCtx.APIVersion)
		if err != nil {
			return nil, err
		}

		items.Value = append(items.Value, versioned)
	}

	return &items, nil
}
