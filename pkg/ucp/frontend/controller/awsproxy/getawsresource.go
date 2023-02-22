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
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

var _ ctrl.Controller = (*GetAWSResource)(nil)

// GetAWSResource is the controller implementation to get AWS resource.
type GetAWSResource struct {
	ctrl.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	AWSOptions
}

// NewGetAWSResource creates a new GetAWSResource.
func NewGetAWSResource(opts ctrl.Options, awsOpts AWSOptions) (ctrl.Controller, error) {
	return &GetAWSResource{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.AWSResource]{},
		),
		awsOpts,
	}, nil
}

func (p *GetAWSResource) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	resourceType, id, err := ParseAWSRequest(ctx, *p.Options(), p.AWSOptions, req)
	if err != nil {
		return nil, err
	}

	response, err := p.AWSOptions.AWSCloudControlClient.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &resourceType,
		Identifier: aws.String(id.Name()),
	})
	if awsclient.IsAWSResourceNotFound(err) {
		return armrpc_rest.NewNotFoundResponse(id), nil
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
		"id":         id.String(),
		"name":       response.ResourceDescription.Identifier,
		"type":       id.Type(),
		"properties": properties,
	}
	return armrpc_rest.NewOKResponse(body), nil
}
