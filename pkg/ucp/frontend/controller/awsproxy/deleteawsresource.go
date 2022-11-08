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
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
)

var _ armrpc_controller.Controller = (*DeleteAWSResource)(nil)

// DeleteAWSResource is the controller implementation to delete AWS resource.
type DeleteAWSResource struct {
	ctrl.BaseController
}

// NewDeleteAWSResource creates a new DeleteAWSResource.
func NewDeleteAWSResource(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &DeleteAWSResource{ctrl.NewBaseController(opts)}, nil
}

func (p *DeleteAWSResource) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	client, resourceType, id, err := ParseAWSRequest(ctx, p.Options, req)
	if err != nil {
		return nil, err
	}

	_, err = client.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &resourceType,
		Identifier: aws.String(id.Name()),
	})
	if awsclient.IsAWSResourceNotFound(err) {
		return armrpc_rest.NewNoContentResponse(), nil
	} else if err != nil {
		return awsclient.HandleAWSError(err)
	}

	response, err := client.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   &resourceType,
		Identifier: aws.String(id.Name()),
	})
	if err != nil {
		return awsclient.HandleAWSError(err)
	}

	operation, err := uuid.Parse(*response.ProgressEvent.RequestToken)
	if err != nil {
		return nil, err
	}

	resp := armrpc_rest.NewAsyncOperationResponse(map[string]interface{}{}, v1.LocationGlobal, 202, id, operation, "", id.RootScope(), p.Options.BasePath)
	return resp, nil
}
