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
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/aws/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

var _ ctrl.Controller = (*DeleteAWSResource)(nil)

// DeleteAWSResource is the controller implementation to delete AWS resource.
type DeleteAWSResource struct {
	ctrl.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	*AWSOptions
}

// NewDeleteAWSResource creates a new DeleteAWSResource.
func NewDeleteAWSResource(awsOpts *AWSOptions) (ctrl.Controller, error) {
	return &DeleteAWSResource{
		ctrl.NewOperation(awsOpts.Options,
			ctrl.ResourceOptions[datamodel.AWSResource]{},
		),
		awsOpts,
	}, nil
}

func (p *DeleteAWSResource) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	// resourceType, id, err := ParseAWSRequest(ctx, p.AWSOptions, req)
	// serviceCtx := servicecontext.AWSRequestContext{}
	// serviceCtx.ResourceID = id
	// serviceCtx.ResourceType = resourceType

	_, err := p.AWSOptions.AWSCloudControlClient.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &serviceCtx.ResourceType,
		Identifier: aws.String(serviceCtx.ResourceID.Name()),
	})
	if awsclient.IsAWSResourceNotFound(err) {
		return armrpc_rest.NewNoContentResponse(), nil
	} else if err != nil {
		return awsclient.HandleAWSError(err)
	}

	response, err := p.AWSOptions.AWSCloudControlClient.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   &serviceCtx.ResourceType,
		Identifier: aws.String(serviceCtx.ResourceID.Name()),
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
