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
	"github.com/project-radius/radius/pkg/to"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	awserror "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/aws/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*DeleteAWSResourceWithPost)(nil)

// DeleteAWSResourceWithPost is the controller implementation to delete an AWS resource.
type DeleteAWSResourceWithPost struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsOptions ctrl.AWSOptions
	basePath   string
}

// NewDeleteAWSResourceWithPost creates a new DeleteAWSResourceWithPost.
func NewDeleteAWSResourceWithPost(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &DeleteAWSResourceWithPost{
		Operation: armrpc_controller.NewOperation(opts.Options,
			armrpc_controller.ResourceOptions[datamodel.AWSResource]{},
		),
		awsOptions: opts.AWSOptions,
		basePath:   opts.BasePath,
	}, nil
}

func (p *DeleteAWSResourceWithPost) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)

	/*Path := req.URL.Path
	region, err := resources.ExtractRegionFromURLPath(Path)
	if err != nil {
		e := v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInvalid,
				Message: "failed to read region from request path",
			},
		}

		response := armrpc_rest.NewBadRequestARMResponse(e)
		err = response.Apply(ctx, w, req)
		if err != nil {
			return nil, err
		}
	}


		cloudControlOpts := []func(*cloudcontrol.Options){}
		cloudControlOpts = append(cloudControlOpts, WithRegion(region))
	*/
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

	describeTypeOutput, err := p.awsOptions.AWSCloudFormationClient.DescribeType(ctx, &cloudformation.DescribeTypeInput{
		Type:     types.RegistryTypeResource,
		TypeName: to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
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

	logger.Info("Deleting resource", "resourceType", serviceCtx.ResourceTypeInAWSFormat(), "resourceID", awsResourceIdentifier)
	response, err := p.awsOptions.AWSCloudControlClient.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
		Identifier: aws.String(awsResourceIdentifier),
	} /*, cloudControlOpts...*/)
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

	resp := armrpc_rest.NewAsyncOperationResponse(map[string]any{}, v1.LocationGlobal, 202, serviceCtx.ResourceID, operation, "", serviceCtx.ResourceID.RootScope(), p.basePath)
	return resp, nil
}
