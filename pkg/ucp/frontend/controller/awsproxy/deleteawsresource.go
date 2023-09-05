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
package awsproxy

import (
	"context"
	http "net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/to"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/servicecontext"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

var _ armrpc_controller.Controller = (*DeleteAWSResource)(nil)

// DeleteAWSResource is the controller implementation to delete AWS resource.
type DeleteAWSResource struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsClients ucp_aws.Clients
}

// NewDeleteAWSResource creates a new DeleteAWSResource controller which is used to delete AWS resources and returns it
// along with a nil error.
func NewDeleteAWSResource(opts armrpc_controller.Options, awsClients ucp_aws.Clients) (armrpc_controller.Controller, error) {
	return &DeleteAWSResource{
		Operation:  armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.AWSResource]{}),
		awsClients: awsClients,
	}, nil
}

// Run() parses the request to get the region, then calls the CloudControl API to delete the resource, and
// returns an AsyncOperationResponse with the operation ID if successful, or an error if not.
func (p *DeleteAWSResource) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	region, errResponse := readRegionFromRequest(req.URL.Path, p.Options().PathBase)
	if errResponse != nil {
		return errResponse, nil
	}

	cloudControlOpts := []func(*cloudcontrol.Options){CloudControlRegionOption(region)}
	response, err := p.awsClients.CloudControl.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
		Identifier: aws.String(serviceCtx.ResourceID.Name()),
	}, cloudControlOpts...)
	if err != nil {
		if ucp_aws.IsAWSResourceNotFoundError(err) {
			return armrpc_rest.NewNoContentResponse(), nil
		}
		return ucp_aws.HandleAWSError(err)
	}

	operation, err := uuid.Parse(*response.ProgressEvent.RequestToken)
	if err != nil {
		return nil, err
	}

	resp := armrpc_rest.NewAsyncOperationResponse(map[string]any{}, v1.LocationGlobal, 202, serviceCtx.ResourceID, operation, "", serviceCtx.ResourceID.RootScope(), p.Options().PathBase)
	return resp, nil
}
