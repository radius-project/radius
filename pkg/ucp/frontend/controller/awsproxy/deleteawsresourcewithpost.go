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
	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	awserror "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*DeleteAWSResourceWithPost)(nil)

// DeleteAWSResourceWithPost is the controller implementation to delete an AWS resource.
type DeleteAWSResourceWithPost struct {
	ctrl.BaseController
}

// NewDeleteAWSResourceWithPost creates a new DeleteAWSResourceWithPost.
func NewDeleteAWSResourceWithPost(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &DeleteAWSResourceWithPost{ctrl.NewBaseController(opts)}, nil
}

func (p *DeleteAWSResourceWithPost) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContext(ctx)
	cloudControlClient, cloudFormationClient, resourceType, id, err := ParseAWSRequest(ctx, p.Options, req)
	if err != nil {
		return nil, err
	}

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

	describeTypeOutput, err := cloudFormationClient.DescribeType(ctx, &cloudformation.DescribeTypeInput{
		Type:     types.RegistryTypeResource,
		TypeName: aws.String(resourceType),
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

	_, err = cloudControlClient.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &resourceType,
		Identifier: aws.String(awsResourceIdentifier),
	})
	if awsclient.IsAWSResourceNotFound(err) {
		return armrpc_rest.NewNoContentResponse(), nil
	} else if err != nil {
		return awsclient.HandleAWSError(err)
	}

	logger.Info("Deleting resource", "resourceType", resourceType, "resourceID", awsResourceIdentifier)
	response, err := cloudControlClient.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   &resourceType,
		Identifier: aws.String(awsResourceIdentifier),
	})
	if err != nil {
		return awsclient.HandleAWSError(err)
	}

	operation, err := uuid.Parse(*response.ProgressEvent.RequestToken)
	if err != nil {
		return nil, err
	}

	resp := armrpc_rest.NewAsyncOperationResponse(map[string]any{}, v1.LocationGlobal, 202, id, operation, "", id.RootScope(), p.Options.BasePath)
	return resp, nil
}
