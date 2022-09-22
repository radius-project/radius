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
	radrprest "github.com/project-radius/radius/pkg/armrpc/rest"
	awserror "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
)

var _ ctrl.Controller = (*DeleteAWSResource)(nil)

// DeleteAWSResource is the controller implementation to delete AWS resource.
type DeleteAWSResource struct {
	ctrl.BaseController
}

// NewDeleteAWSResource creates a new DeleteAWSResource.
func NewDeleteAWSResource(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteAWSResource{ctrl.NewBaseController(opts)}, nil
}

func (p *DeleteAWSResource) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	client, resourceType, id, err := ParseAWSRequest(ctx, p.Options.BasePath, req)
	if err != nil {
		return nil, err
	}

	_, err = client.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &resourceType,
		Identifier: aws.String(id.Name()),
	})
	if awserror.IsAWSResourceNotFound(err) {
		return rest.NewNoContentResponse(), nil
	} else if err != nil {
		return awserror.HandleAWSError(err)
	}

	response, err := client.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   &resourceType,
		Identifier: aws.String(id.Name()),
	})
	if err != nil {
		return awserror.HandleAWSError(err)
	}

	operation, err := uuid.Parse(*response.ProgressEvent.RequestToken)
	if err != nil {
		return nil, err
	}

	resp := radrprest.NewAsyncOperationResponse(map[string]interface{}{}, "global", 202, id, operation, "", id.RootScope(), p.Options.BasePath)
	return resp, nil
}
