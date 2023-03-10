// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"context"
	"encoding/json"
	http "net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/aws/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
)

var _ armrpc_controller.Controller = (*GetAWSResource)(nil)

// GetAWSResource is the controller implementation to get AWS resource.
type GetAWSResource struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsOptions ctrl.AWSOptions
}

// NewGetAWSResource creates a new GetAWSResource.
func NewGetAWSResource(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &GetAWSResource{
		Operation: armrpc_controller.NewOperation(opts.Options,
			armrpc_controller.ResourceOptions[datamodel.AWSResource]{},
		),
		awsOptions: opts.AWSOptions,
	}, nil
}

func (p *GetAWSResource) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)

	response, err := p.awsOptions.AWSCloudControlClient.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &serviceCtx.ResourceType,
		Identifier: aws.String(serviceCtx.ResourceID.Name()),
	})
	if awsclient.IsAWSResourceNotFound(err) {
		return armrpc_rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	} else if err != nil {
		return awsclient.HandleAWSError(err)
	}

	properties := map[string]any{}
	if response.ResourceDescription.Properties != nil {
		err := json.Unmarshal([]byte(*response.ResourceDescription.Properties), &properties)
		if err != nil {
			return nil, err
		}
	}

	body := map[string]any{
		"id":         serviceCtx.ResourceID.String(),
		"name":       response.ResourceDescription.Identifier,
		"type":       serviceCtx.ResourceID.Type(),
		"properties": properties,
	}
	return armrpc_rest.NewOKResponse(body), nil
}
