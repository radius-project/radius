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
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	awsoperations "github.com/project-radius/radius/pkg/aws/operations"
	awserror "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/aws/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

var _ ctrl.Controller = (*CreateOrUpdateAWSResourceWithPost)(nil)

// CreateOrUpdateAWSResourceWithPost is the controller implementation to create/update an AWS resource.
type CreateOrUpdateAWSResourceWithPost struct {
	ctrl.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	*AWSOptions
}

// NewCreateOrUpdateAWSResourceWithPost creates a new CreateOrUpdateAWSResourceWithPost.
func NewCreateOrUpdateAWSResourceWithPost(awsOpts *AWSOptions) (ctrl.Controller, error) {
	return &CreateOrUpdateAWSResourceWithPost{
		ctrl.NewOperation(awsOpts.Options,
			ctrl.ResourceOptions[datamodel.AWSResource]{},
		),
		awsOpts,
	}, nil
}

func (p *CreateOrUpdateAWSResourceWithPost) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := logr.FromContextOrDiscard(ctx)
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)

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

	describeTypeOutput, err := p.AWSOptions.AWSCloudFormationClient.DescribeType(ctx, &cloudformation.DescribeTypeInput{
		Type:     types.RegistryTypeResource,
		TypeName: aws.String(serviceCtx.ResourceType),
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
		computedResourceID = computeResourceID(serviceCtx.ResourceID, awsResourceIdentifier)

		// Create and update work differently for AWS - we need to know if the resource
		// we're working on exists already.
		getResponse, err = p.AWSOptions.AWSCloudControlClient.GetResource(ctx, &cloudcontrol.GetResourceInput{
			TypeName:   &serviceCtx.ResourceType,
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
		logger.Info("Updating resource", "resourceType", serviceCtx.ResourceType, "resourceID", awsResourceIdentifier)

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

			response, err := p.AWSOptions.AWSCloudControlClient.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{
				TypeName:      &serviceCtx.ResourceType,
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
				"type":       serviceCtx.ResourceID.Type(),
				"properties": responseProperties,
			}

			resp := armrpc_rest.NewOKResponse(responseBody)
			return resp, nil
		}
	} else {
		logger.Info("Creating resource", "resourceType", serviceCtx.ResourceType, "resourceID", awsResourceIdentifier)
		response, err := p.AWSOptions.AWSCloudControlClient.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
			TypeName:     &serviceCtx.ResourceType,
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

	resp := armrpc_rest.NewAsyncOperationResponse(responseBody, v1.LocationGlobal, 201, serviceCtx.ResourceID, operation, "", serviceCtx.ResourceID.RootScope(), p.AWSOptions.Options.BasePath)
	return resp, nil
}
