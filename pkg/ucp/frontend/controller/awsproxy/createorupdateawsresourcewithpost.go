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
	"errors"
	"fmt"
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
	awserror "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/aws/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*CreateOrUpdateAWSResourceWithPost)(nil)

// CreateOrUpdateAWSResourceWithPost is the controller implementation to create/update an AWS resource.
type CreateOrUpdateAWSResourceWithPost struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsOptions ctrl.AWSOptions
	basePath   string
}

// NewCreateOrUpdateAWSResourceWithPost creates a new CreateOrUpdateAWSResourceWithPost.
func NewCreateOrUpdateAWSResourceWithPost(opts ctrl.Options) (armrpc_controller.Controller, error) {

	return &CreateOrUpdateAWSResourceWithPost{
		Operation: armrpc_controller.NewOperation(opts.Options,
			armrpc_controller.ResourceOptions[datamodel.AWSResource]{},
		),
		awsOptions: opts.AWSOptions,
		basePath:   opts.BasePath,
	}, nil
}

func (p *CreateOrUpdateAWSResourceWithPost) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	region, errResponse := readRegionFromRequest(req.URL.Path, p.basePath)
	if errResponse != nil {
		return errResponse, nil
	}

	properties, err := readPropertiesFromBody(req)
	if err != nil {
		e := v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInvalid,
				Message: "failed to read request body",
			},
		}
		return armrpc_rest.NewBadRequestARMResponse(e), nil
	}

	cloudControlOpts := []func(*cloudcontrol.Options){CloudControlRegionOption(region)}
	cloudFormationOpts := []func(*cloudformation.Options){CloudFormationWithRegionOption(region)}

	describeTypeOutput, err := p.awsOptions.AWSCloudFormationClient.DescribeType(ctx, &cloudformation.DescribeTypeInput{
		Type:     types.RegistryTypeResource,
		TypeName: to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
	}, cloudFormationOpts...)
	if err != nil {
		return awserror.HandleAWSError(err)
	}

	var operation uuid.UUID
	desiredState, err := json.Marshal(properties)
	if err != nil {
		return awserror.HandleAWSError(err)
	}

	existing := true
	var getResponse *cloudcontrol.GetResourceOutput = nil
	computedResourceID := ""
	responseProperties := map[string]any{}

	awsResourceIdentifier, err := getPrimaryIdentifierFromMultiIdentifiers(ctx, properties, *describeTypeOutput.Schema)
	if errors.Is(&awserror.AWSMissingPropertyError{}, err) {
		// assume that if we can't get the AWS resource identifier, we need to create the resource
		existing = false
	} else if err != nil {
		return awserror.HandleAWSError(err)
	} else {
		computedResourceID = computeResourceID(serviceCtx.ResourceID, awsResourceIdentifier)

		// Create and update work differently for AWS - we need to know if the resource
		// we're working on exists already.
		getResponse, err = p.awsOptions.AWSCloudControlClient.GetResource(ctx, &cloudcontrol.GetResourceInput{
			TypeName:   to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
			Identifier: aws.String(awsResourceIdentifier),
		}, cloudControlOpts...)
		if awserror.IsAWSResourceNotFoundError(err) {
			existing = false
		} else if err != nil {
			return awserror.HandleAWSError(err)
		} else {
			err = json.Unmarshal([]byte(*getResponse.ResourceDescription.Properties), &responseProperties)
			if err != nil {
				return awserror.HandleAWSError(err)
			}
		}
	}

	// Properties specified by users take precedence
	for k, v := range properties {
		responseProperties[k] = v
	}

	if existing {
		logger.Info(fmt.Sprintf("Updating resource : resourceType %q resourceID %q", serviceCtx.ResourceTypeInAWSFormat(), awsResourceIdentifier))

		// Generate patch
		currentState := []byte(*getResponse.ResourceDescription.Properties)
		resourceTypeSchema := []byte(*describeTypeOutput.Schema)
		patch, err := awsoperations.GeneratePatch(currentState, desiredState, resourceTypeSchema)
		if err != nil {
			return awserror.HandleAWSError(err)
		}

		// Call update only if the patch is not empty
		if len(patch) > 0 {
			marshaled, err := json.Marshal(&patch)
			if err != nil {
				return awserror.HandleAWSError(err)
			}

			response, err := p.awsOptions.AWSCloudControlClient.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{
				TypeName:      to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
				Identifier:    aws.String(awsResourceIdentifier),
				PatchDocument: aws.String(string(marshaled)),
			}, cloudControlOpts...)
			if err != nil {
				return awserror.HandleAWSError(err)
			}

			operation, err = uuid.Parse(*response.ProgressEvent.RequestToken)
			if err != nil {
				return awserror.HandleAWSError(err)
			}
		} else {
			// mark provisioning state as succeeded here
			// and return 200, telling the deployment engine that the resource has already been created
			responseProperties["provisioningState"] = v1.ProvisioningStateSucceeded
			responseBody := map[string]any{
				"id":         computedResourceID,
				"name":       awsResourceIdentifier,
				"type":       serviceCtx.ResourceID.Type(),
				"properties": responseProperties,
			}

			resp := armrpc_rest.NewOKResponse(responseBody)
			return resp, nil
		}
	} else {
		logger.Info(fmt.Sprintf("Creating resource : resourceType %q resourceID %q", serviceCtx.ResourceTypeInAWSFormat(), awsResourceIdentifier))
		response, err := p.awsOptions.AWSCloudControlClient.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
			TypeName:     to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
			DesiredState: aws.String(string(desiredState)),
		}, cloudControlOpts...)
		if err != nil {
			return awserror.HandleAWSError(err)
		}

		operation, err = uuid.Parse(*response.ProgressEvent.RequestToken)
		if err != nil {
			return awserror.HandleAWSError(err)
		}

		// Get the resource identifier from the progress event response
		if response != nil && response.ProgressEvent != nil && response.ProgressEvent.Identifier != nil {
			awsResourceIdentifier = *response.ProgressEvent.Identifier
			computedResourceID = computeResourceID(serviceCtx.ResourceID, awsResourceIdentifier)
		}
	}

	responseProperties["provisioningState"] = v1.ProvisioningStateProvisioning

	responseBody := map[string]any{
		"type":       serviceCtx.ResourceID.Type(),
		"properties": responseProperties,
	}
	if computedResourceID != "" && awsResourceIdentifier != "" {
		responseBody["id"] = computedResourceID
		responseBody["name"] = awsResourceIdentifier
	}

	resp := armrpc_rest.NewAsyncOperationResponse(responseBody, v1.LocationGlobal, 201, serviceCtx.ResourceID, operation, "", serviceCtx.ResourceID.RootScope(), p.basePath)
	return resp, nil
}
