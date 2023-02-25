// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"context"
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
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	awserror "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/aws/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

var _ ctrl.Controller = (*DeleteAWSResourceWithPost)(nil)

// DeleteAWSResourceWithPost is the controller implementation to delete an AWS resource.
type DeleteAWSResourceWithPost struct {
	ctrl.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	*AWSOptions
}

// NewDeleteAWSResourceWithPost creates a new DeleteAWSResourceWithPost.
func NewDeleteAWSResourceWithPost(awsOpts *AWSOptions) (ctrl.Controller, error) {
	return &DeleteAWSResourceWithPost{
		ctrl.NewOperation(awsOpts.Options,
			ctrl.ResourceOptions[datamodel.AWSResource]{},
		),
		awsOpts,
	}, nil
}

func (p *DeleteAWSResourceWithPost) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := logr.FromContextOrDiscard(ctx)
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)

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

	describeTypeOutput, err := p.AWSOptions.AWSCloudFormationClient.DescribeType(ctx, &cloudformation.DescribeTypeInput{
		Type:     types.RegistryTypeResource,
		TypeName: aws.String(serviceCtx.ResourceType),
	})
	if err != nil {
		return awserror.HandleAWSError(err)
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

	_, err = p.AWSOptions.AWSCloudControlClient.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &serviceCtx.ResourceType,
		Identifier: aws.String(awsResourceIdentifier),
	})
	if awsclient.IsAWSResourceNotFound(err) {
		return armrpc_rest.NewNoContentResponse(), nil
	} else if err != nil {
		return awsclient.HandleAWSError(err)
	}

	logger.Info("Deleting resource", "resourceType", serviceCtx.ResourceType, "resourceID", awsResourceIdentifier)
	response, err := p.AWSOptions.AWSCloudControlClient.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   &serviceCtx.ResourceType,
		Identifier: aws.String(awsResourceIdentifier),
	})
	if err != nil {
		return awsclient.HandleAWSError(err)
	}

	operation, err := uuid.Parse(*response.ProgressEvent.RequestToken)
	if err != nil {
		return nil, err
	}

	resp := armrpc_rest.NewAsyncOperationResponse(map[string]any{}, v1.LocationGlobal, 202, serviceCtx.ResourceID, operation, "", serviceCtx.ResourceID.RootScope(), p.AWSOptions.Options.BasePath)
	return resp, nil
}
