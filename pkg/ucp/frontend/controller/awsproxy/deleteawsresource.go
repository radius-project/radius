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
	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/to"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/aws/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var _ armrpc_controller.Controller = (*DeleteAWSResource)(nil)

// DeleteAWSResource is the controller implementation to delete AWS resource.
type DeleteAWSResource struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsOptions ctrl.AWSOptions
	basePath   string
}

// NewDeleteAWSResource creates a new DeleteAWSResource.
func NewDeleteAWSResource(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &DeleteAWSResource{
		Operation: armrpc_controller.NewOperation(opts.Options,
			armrpc_controller.ResourceOptions[datamodel.AWSResource]{},
		),
		awsOptions: opts.AWSOptions,
		basePath:   opts.BasePath,
	}, nil
}

func (p *DeleteAWSResource) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)

	path := req.URL.Path
	region, err := resources.ExtractRegionFromURLPath(path)
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

	response, err := p.awsOptions.AWSCloudControlClient.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
		Identifier: aws.String(serviceCtx.ResourceID.Name()),
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

	resp := armrpc_rest.NewAsyncOperationResponse(map[string]any{}, v1.LocationGlobal, 202, serviceCtx.ResourceID, operation, "", serviceCtx.ResourceID.RootScope(), p.basePath)
	return resp, nil
}
