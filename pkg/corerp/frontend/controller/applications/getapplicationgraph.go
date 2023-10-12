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
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/resources"

	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
)

var (
	schemePrefix = "http://"
)

var _ ctrl.Controller = (*GetApplicationGraph)(nil)

// GetApplicationGraph is the controller implementation to get application graph.
type GetApplicationGraph struct {
	ctrl.Operation[*datamodel.Application, datamodel.Application]
	conn sdk.Connection
}

// NewGetApplicationGraph creates a new instance of the GetApplicationGraph controller.
func NewGetApplicationGraph(opts ctrl.Options, conn sdk.Connection) (ctrl.Controller, error) {
	return &GetApplicationGraph{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Application]{
				RequestConverter:  converter.ApplicationDataModelFromVersioned,
				ResponseConverter: converter.ApplicationDataModelToVersioned,
			},
		),
		conn,
	}, nil
}

func (ctrl *GetApplicationGraph) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
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
	environmentID, err := resources.Parse(applicationResource.Properties.Environment)
	if err != nil {
		return nil, err
	}

	clientOptions := sdk.NewClientOptions(ctrl.conn)

	// get all resources in application scope
	applicationResources, err := listAllResourcesByApplication(ctx, applicationID, clientOptions)
	if err != nil {
		return nil, err
	}

	// get all resources in environment scope
	environmentResources, err := listAllResourcesByEnvironment(ctx, environmentID, clientOptions)
	if err != nil {
		return nil, err
	}

	graph := computeGraph(applicationID.Name(), applicationResources, environmentResources)
	if err != nil {
		return nil, err
	}
	return rest.NewOKResponse(graph), nil
}
