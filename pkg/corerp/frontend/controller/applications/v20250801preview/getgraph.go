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

package v20250801preview

import (
	"context"
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	app_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/applications"
	"github.com/radius-project/radius/pkg/sdk"
)

var _ ctrl.Controller = (*GetGraphv20250801preview)(nil)

// GetGraphv20250801preview is the controller implementation to get the application graph for
// Radius.Core/applications resources.
type GetGraphv20250801preview struct {
	ctrl.Operation[*datamodel.Application_v20250801preview, datamodel.Application_v20250801preview]
	connection sdk.Connection
}

// NewGetGraphv20250801preview creates a new instance of the GetGraphv20250801preview controller.
func NewGetGraphv20250801preview(opts ctrl.Options, connection sdk.Connection) (ctrl.Controller, error) {
	return &GetGraphv20250801preview{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Application_v20250801preview]{
				RequestConverter:  converter.Application20250801DataModelFromVersioned,
				ResponseConverter: converter.Application20250801DataModelToVersioned,
			},
		),
		connection,
	}, nil
}

// Run handles the getGraph custom action for Radius.Core/applications. It looks up the application,
// resolves its environment, lists application- and environment-scoped resources, and returns the
// computed application graph.
func (ctrl *GetGraphv20250801preview) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	// Request route for getGraph has the operation name as suffix which must be removed to get the resource id.
	// route id format: /planes/radius/local/resourcegroups/default/providers/Radius.Core/applications/<app>/getGraph
	applicationID := sCtx.ResourceID.Truncate()
	applicationResource, _, err := ctrl.GetResource(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	if applicationResource == nil {
		return rest.NewNotFoundResponse(sCtx.ResourceID), nil
	}

	// An application **MUST** have an environment id.
	return app_ctrl.ComputeGraphResponse(ctx, applicationID, applicationResource.Properties.Environment, ctrl.connection)
}
