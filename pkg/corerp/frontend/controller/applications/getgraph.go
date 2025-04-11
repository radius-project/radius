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
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/resources"

	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	ucp_v20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

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
	environmentID, err := resources.Parse(applicationResource.Properties.Environment)
	if err != nil {
		return nil, err
	}

	clientOptions := sdk.NewClientOptions(ctrl.connection)

	clientFactory, err := ucp_v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("error creating client factory: %w", err)
	}

	rpc := clientFactory.NewResourceProvidersClient()

	resourceProviders := rpc.NewListProviderSummariesPager("local", nil)

	rpSummaries := []*ucp_v20231001preview.ResourceProviderSummary{}

	// Get the list of all resource providers
	//resourceProvidersList := make([]ucp_v20231001preview.ResourceProvidersClientListProviderSummariesResponse, 0)
	for resourceProviders.More() {
		page, err := resourceProviders.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("error getting resource providers: %w", err)
		}

		rpSummaries = append(rpSummaries, page.Value...)
	}

	// Get the list of all resource providers
	fmt.Print(rpSummaries)

	// Get the list of all resource type in form rp/rt
	for _, rpSummary := range rpSummaries {
		for resourceType, _ := range rpSummary.ResourceTypes {
			fmt.Printf("Resource Type: %s/%s\n", *rpSummary.Name, resourceType)

		}
	}

	applicationResources, err := listAllResourcesByApplication(ctx, applicationID, clientOptions)
	if err != nil {
		return nil, err
	}

	environmentResources, err := listAllResourcesByEnvironment(ctx, environmentID, clientOptions)
	if err != nil {
		return nil, err
	}

	graph := computeGraph(applicationResources, environmentResources)
	return rest.NewOKResponse(graph), nil
}
