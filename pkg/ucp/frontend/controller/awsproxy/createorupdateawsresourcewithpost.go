// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"context"
	"encoding/json"
	"errors"
	http "net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	awsoperations "github.com/project-radius/radius/pkg/aws/operations"
	awserror "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
)

var _ armrpc_controller.Controller = (*CreateOrUpdateAWSResourceWithPost)(nil)

// CreateOrUpdateAWSResourceWithPost is the controller implementation to create/update an AWS resource.
type CreateOrUpdateAWSResourceWithPost struct {
	ctrl.BaseController
}

// NewCreateOrUpdateAWSResourceWithPost creates a new CreateOrUpdateAWSResourceWithPost.
func NewCreateOrUpdateAWSResourceWithPost(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &CreateOrUpdateAWSResourceWithPost{ctrl.NewBaseController(opts)}, nil
}

func (p *CreateOrUpdateAWSResourceWithPost) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := logr.FromContextOrDiscard(ctx)
	cloudControlClient, cloudFormationClient, resourceType, id, err := ParseAWSRequest(ctx, p.Options, req)
	if err != nil {
		return nil, err
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

	describeTypeOutput, err := cloudFormationClient.DescribeType(ctx, &cloudformation.DescribeTypeInput{
		Type:     types.RegistryTypeResource,
		TypeName: aws.String(resourceType),
	})
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
		computedResourceID = computeResourceID(id, awsResourceIdentifier)

		// Create and update work differently for AWS - we need to know if the resource
		// we're working on exists already.
		getResponse, err = cloudControlClient.GetResource(ctx, &cloudcontrol.GetResourceInput{
			TypeName:   &resourceType,
			Identifier: aws.String(awsResourceIdentifier),
		})
		if awserror.IsAWSResourceNotFound(err) {
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
		logger.Info("Updating resource", "resourceType", resourceType, "resourceID", awsResourceIdentifier)

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

			response, err := cloudControlClient.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{
				TypeName:      &resourceType,
				Identifier:    aws.String(awsResourceIdentifier),
				PatchDocument: aws.String(string(marshaled)),
			})
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
				"type":       id.Type(),
				"properties": responseProperties,
			}

			resp := armrpc_rest.NewOKResponse(responseBody)
			return resp, nil
		}
	} else {
		logger.Info("Creating resource", "resourceType", resourceType, "resourceID", awsResourceIdentifier)
		response, err := cloudControlClient.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
			TypeName:     &resourceType,
			DesiredState: aws.String(string(desiredState)),
		})
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
			computedResourceID = computeResourceID(id, awsResourceIdentifier)
		}
	}

	responseProperties["provisioningState"] = v1.ProvisioningStateProvisioning

	responseBody := map[string]any{
		"type":       id.Type(),
		"properties": responseProperties,
	}
	if computedResourceID != "" && awsResourceIdentifier != "" {
		responseBody["id"] = computedResourceID
		responseBody["name"] = awsResourceIdentifier
	}

	resp := armrpc_rest.NewAsyncOperationResponse(responseBody, v1.LocationGlobal, 201, id, operation, "", id.RootScope(), p.Options.BasePath)
	return resp, nil
}
