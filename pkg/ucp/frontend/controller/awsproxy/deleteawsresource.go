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
	awstypes "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
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
	resourceType := ctx.Value(AWSResourceTypeKey).(string)
	client := ctx.Value(AWSClientKey).(awstypes.AWSClient)
	id := ctx.Value(AWSResourceID).(resources.ID)

	_, err := client.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &resourceType,
		Identifier: aws.String(id.Name()),
	})
	if awstypes.IsAWSResourceNotFound(err) {
		return rest.NewNoContentResponse(), nil
	} else if err != nil {
		return awstypes.HandleAWSError(err)
	}

	response, err := client.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   &resourceType,
		Identifier: aws.String(id.Name()),
	})
	if err != nil {
		return awstypes.HandleAWSError(err)
	}

	operation, err := uuid.Parse(*response.ProgressEvent.RequestToken)
	if err != nil {
		return nil, err
	}

	resp := radrprest.NewAsyncOperationResponse(map[string]interface{}{}, "global", 202, id, operation, "")
	resp.(*radrprest.AsyncOperationResponse).RootScope = id.RootScope()
	resp.(*radrprest.AsyncOperationResponse).PathBase = p.Options.BasePath
	return resp, nil
}
