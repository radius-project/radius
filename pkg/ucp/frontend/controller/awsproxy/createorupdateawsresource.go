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
	"encoding/json"
	http "net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	awsoperations "github.com/project-radius/radius/pkg/aws/operations"
	"github.com/project-radius/radius/pkg/to"
	ucp_aws "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/aws/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

var _ armrpc_controller.Controller = (*CreateOrUpdateAWSResource)(nil)

// CreateOrUpdateAWSResource is the controller implementation to create/update an AWS resource.
type CreateOrUpdateAWSResource struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsClients ucp_aws.Clients
}

// NewCreateOrUpdateAWSResource creates a new CreateOrUpdateAWSResource.
func NewCreateOrUpdateAWSResource(opts armrpc_controller.Options, awsClients ucp_aws.Clients) (armrpc_controller.Controller, error) {
	return &CreateOrUpdateAWSResource{
		armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.AWSResource]{}),
		awsClients,
	}, nil
}

// # Function Explanation
//
// "Run" reads the request body, determines if the resource exists, and either creates or updates the
// resource accordingly, returning an async operation response with the resource's properties and a request token.
func (p *CreateOrUpdateAWSResource) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)

	decoder := json.NewDecoder(req.Body)
	defer req.Body.Close()
	region, errResponse := readRegionFromRequest(req.URL.Path, p.Options().PathBase)
	if errResponse != nil {
		return errResponse, nil
	}

	body := map[string]any{}
	err := decoder.Decode(&body)
	if err != nil {
		e := v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInvalid,
				Message: "failed to read request body",
			},
		}

		response := armrpc_rest.NewBadRequestARMResponse(e)
		return response, nil
	}

	properties := map[string]any{}
	obj, ok := body["properties"]
	if ok {
		pp, ok := obj.(map[string]any)
		if ok {
			properties = pp
		}
	}

	cloudControlOpts := []func(*cloudcontrol.Options){CloudControlRegionOption(region)}
	cloudFormationOpts := []func(*cloudformation.Options){CloudFormationWithRegionOption(region)}

	// Create and update work differently for AWS - we need to know if the resource
	// we're working on exists already.
	existing := true
	getResponse, err := p.awsClients.CloudControl.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
		Identifier: aws.String(serviceCtx.ResourceID.Name()),
	}, cloudControlOpts...)
	if ucp_aws.IsAWSResourceNotFoundError(err) {
		existing = false
	} else if err != nil {
		return ucp_aws.HandleAWSError(err)
	}

	var operation uuid.UUID
	desiredState, err := json.Marshal(properties)
	if err != nil {
		return ucp_aws.HandleAWSError(err)
	}

	// AWS doesn't return the resource state as part of the cloud-control operation. Let's
	// simulate that here.
	responseProperties := map[string]any{}
	if getResponse != nil {
		err = json.Unmarshal([]byte(*getResponse.ResourceDescription.Properties), &responseProperties)
		if err != nil {
			return ucp_aws.HandleAWSError(err)
		}
	}

	// Properties specified by users take precedence
	for k, v := range properties {
		responseProperties[k] = v
	}

	if existing {
		// Get resource type schema

		describeTypeOutput, err := p.awsClients.CloudFormation.DescribeType(ctx, &cloudformation.DescribeTypeInput{
			Type:     types.RegistryTypeResource,
			TypeName: to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
		}, cloudFormationOpts...)
		if err != nil {
			return nil, err
		}

		// Generate patch
		currentState := []byte(*getResponse.ResourceDescription.Properties)
		resourceTypeSchema := []byte(*describeTypeOutput.Schema)
		patch, err := awsoperations.GeneratePatch(currentState, desiredState, resourceTypeSchema)
		if err != nil {
			return ucp_aws.HandleAWSError(err)
		}

		// Call update only if the patch is not empty
		if len(patch) > 0 {
			marshaled, err := json.Marshal(&patch)
			if err != nil {
				return ucp_aws.HandleAWSError(err)
			}

			response, err := p.awsClients.CloudControl.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{
				TypeName:      to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
				Identifier:    aws.String(serviceCtx.ResourceID.Name()),
				PatchDocument: aws.String(string(marshaled)),
			}, cloudControlOpts...)
			if err != nil {
				return ucp_aws.HandleAWSError(err)
			}

			operation, err = uuid.Parse(*response.ProgressEvent.RequestToken)
			if err != nil {
				return ucp_aws.HandleAWSError(err)
			}
		} else {
			// mark provisioning state as succeeded here
			// and return 200, telling the deployment engine that the resource has already been created
			responseProperties["provisioningState"] = v1.ProvisioningStateSucceeded
			responseBody := map[string]any{
				"id":         serviceCtx.ResourceID.String(),
				"name":       serviceCtx.ResourceID.Name(),
				"type":       serviceCtx.ResourceID.Type(),
				"properties": responseProperties,
			}

			resp := armrpc_rest.NewOKResponse(responseBody)
			return resp, nil
		}
	} else {
		response, err := p.awsClients.CloudControl.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
			TypeName:     to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
			DesiredState: aws.String(string(desiredState)),
		}, cloudControlOpts...)
		if err != nil {
			return ucp_aws.HandleAWSError(err)
		}

		operation, err = uuid.Parse(*response.ProgressEvent.RequestToken)
		if err != nil {
			return ucp_aws.HandleAWSError(err)
		}
	}

	responseProperties["provisioningState"] = v1.ProvisioningStateProvisioning

	responseBody := map[string]any{
		"id":         serviceCtx.ResourceID.String(),
		"name":       serviceCtx.ResourceID.Name(),
		"type":       serviceCtx.ResourceID.Type(),
		"properties": responseProperties,
	}

	resp := armrpc_rest.NewAsyncOperationResponse(responseBody, v1.LocationGlobal, 201, serviceCtx.ResourceID, operation, "", serviceCtx.ResourceID.RootScope(), p.Options().PathBase)
	return resp, nil
}
