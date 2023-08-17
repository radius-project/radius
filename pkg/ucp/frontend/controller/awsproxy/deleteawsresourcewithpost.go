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
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/to"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	ucp_aws "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/aws/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*DeleteAWSResourceWithPost)(nil)

// DeleteAWSResourceWithPost is the controller implementation to delete an AWS resource.
type DeleteAWSResourceWithPost struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsClients ucp_aws.Clients
}

// NewDeleteAWSResourceWithPost creates a new DeleteAWSResourceWithPost.
//

// NewDeleteAWSResourceWithPost creates a new DeleteAWSResourceWithPost controller which is used to delete an AWS resource
// using a POST request.
func NewDeleteAWSResourceWithPost(opts armrpc_controller.Options, awsClients ucp_aws.Clients) (armrpc_controller.Controller, error) {
	return &DeleteAWSResourceWithPost{
		Operation:  armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.AWSResource]{}),
		awsClients: awsClients,
	}, nil
}

// Run() reads the region from the request, reads properties from the body, gets the primary
// identifier from the properties, logs the resource to be deleted, deletes the resource, and returns an async operation
// response. If the resource is not found, it returns a no content response. If an error occurs, it returns an error response.
func (p *DeleteAWSResourceWithPost) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	region, errResponse := readRegionFromRequest(req.URL.Path, p.Options().PathBase)
	if errResponse != nil {
		return errResponse, nil
	}

	cloudControlOpts := []func(*cloudcontrol.Options){CloudControlRegionOption(region)}
	properties, err := readPropertiesFromBody(req)
	if err != nil {
		e := v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInvalid,
				Message: err.Error(),
			},
		}
		return armrpc_rest.NewBadRequestARMResponse(e), nil
	}

	cloudFormationOpts := []func(*cloudformation.Options){CloudFormationWithRegionOption(region)}
	describeTypeOutput, err := p.awsClients.CloudFormation.DescribeType(ctx, &cloudformation.DescribeTypeInput{
		Type:     types.RegistryTypeResource,
		TypeName: to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
	}, cloudFormationOpts...)
	if err != nil {
		return ucp_aws.HandleAWSError(err)
	}

	awsResourceIdentifier, err := getPrimaryIdentifierFromMultiIdentifiers(ctx, properties, *describeTypeOutput.Schema)
	if err != nil {
		e := v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInvalid,
				Message: err.Error(),
			},
		}
		return armrpc_rest.NewBadRequestARMResponse(e), nil
	}

	logger.Info("Deleting resource", "resourceType", serviceCtx.ResourceTypeInAWSFormat(), "resourceID", awsResourceIdentifier)
	response, err := p.awsClients.CloudControl.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
		Identifier: aws.String(awsResourceIdentifier),
	}, cloudControlOpts...)
	if err != nil {
		if awsclient.IsAWSResourceNotFoundError(err) {
			return armrpc_rest.NewNoContentResponse(), nil
		}
		return awsclient.HandleAWSError(err)
	}

	operation, err := uuid.Parse(*response.ProgressEvent.RequestToken)
	if err != nil {
		return nil, err
	}

	resp := armrpc_rest.NewAsyncOperationResponse(map[string]any{}, v1.LocationGlobal, 202, serviceCtx.ResourceID, operation, "", serviceCtx.ResourceID.RootScope(), p.Options().PathBase)
	return resp, nil
}
