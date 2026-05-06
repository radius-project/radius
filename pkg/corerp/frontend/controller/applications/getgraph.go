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

package applications

import (
	"context"
	"fmt"
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/resources"

	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
)

const (
	radiusPlane = "/planes/radius/"
	planeName   = "local"
)

// ComputeGraphResponse computes the application graph for the given application and environment IDs and
// returns it wrapped in an OK rest.Response. It is shared by the Applications.Core and Radius.Core
// implementations of the getGraph custom action.
func ComputeGraphResponse(ctx context.Context, applicationID resources.ID, environmentIDString string, connection sdk.Connection) (rest.Response, error) {
	// An application **MUST** have an environment id
	environmentID, err := resources.ParseResource(environmentIDString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse environment ID %q: %w", environmentIDString, err)
	}

	clientOptions := sdk.NewClientOptions(connection)

	ucpApplicationsManagementClient := &clients.UCPApplicationsManagementClient{
		RootScope:     radiusPlane + planeName,
		ClientOptions: clientOptions,
	}

	resourceTypes, err := ucpApplicationsManagementClient.ListAllResourceTypesNames(ctx, "local")
	if err != nil {
		return nil, err
	}

	applicationResources, err := listAllResourcesByApplication(ctx, applicationID, resourceTypes, clientOptions)
	if err != nil {
		return nil, err
	}

	environmentResources, err := listAllResourcesByEnvironment(ctx, environmentID, resourceTypes, clientOptions)
	if err != nil {
		return nil, err
	}

	graph := computeGraph(applicationResources, environmentResources)
	return rest.NewOKResponse(graph), nil
}

var _ ctrl.Controller = (*GetGraph)(nil)

// GetGraph is the controller implementation to get application graph.
type GetGraph struct {
	ctrl.Operation[*datamodel.Application, datamodel.Application]
	connection sdk.Connection
}

// NewGetGraph creates a new instance of the GetGraph controller.
func NewGetGraph(opts ctrl.Options, connection sdk.Connection) (ctrl.Controller, error) {
	return &GetGraph{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Application]{
				RequestConverter:  converter.ApplicationDataModelFromVersioned,
				ResponseConverter: converter.ApplicationDataModelToVersioned,
			},
		),
		connection,
	}, nil
}

func (ctrl *GetGraph) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	// Request route for getGraph has name of the operation as suffix which should be removed to get the resource id.
	// route id format: /planes/radius/local/resourcegroups/default/providers/Applications.Core/applications/corerp-resources-application-app/getGraph"
	applicationID := sCtx.ResourceID.Truncate()
	applicationResource, _, err := ctrl.GetResource(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	if applicationResource == nil {
		return rest.NewNotFoundResponse(sCtx.ResourceID), nil
	}
	// An application **MUST** have an environment id
	return ComputeGraphResponse(ctx, applicationID, applicationResource.Properties.Environment, ctrl.connection)
}
