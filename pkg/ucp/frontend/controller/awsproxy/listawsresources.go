// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"context"
	"encoding/json"
	http "net/http"
	"path"

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

var _ ctrl.Controller = (*ListAWSResources)(nil)

// ListAWSResources is the controller implementation to get/list AWS resources.
type ListAWSResources struct {
	ctrl.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	AWSOptions
}

// NewListAWSResources creates a new ListAWSResources.
func NewListAWSResources(opts ctrl.Options, awsOpts AWSOptions) (ctrl.Controller, error) {
	return &ListAWSResources{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.AWSResource]{},
		),
		awsOpts,
	}, nil
}

func (p *ListAWSResources) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	resourceType, id, err := ParseAWSRequest(ctx, *p.Options(), p.AWSOptions, req)
	if err != nil {
		return nil, err
	}

	// TODO pagination
	response, err := p.AWSOptions.AWSCloudControlClient.ListResources(ctx, &cloudcontrol.ListResourcesInput{
		TypeName: &resourceType,
	})
	if err != nil {
		return awsclient.HandleAWSError(err)
	}

	// TODO there some limitations with listing resources:
	//
	// https://docs.aws.amazon.com/cloudcontrolapi/latest/userguide/resource-operations-list.html

	items := []any{}
	for _, result := range response.ResourceDescriptions {
		properties := map[string]any{}
		if result.Properties != nil {
			err := json.Unmarshal([]byte(*result.Properties), &properties)
			if err != nil {
				return nil, err
			}
		}

		resourceName := *result.Identifier
		item := map[string]any{
			"id":         path.Join(id.String(), resourceName),
			"name":       result.Identifier,
			"type":       id.Type(),
			"properties": properties,
		}
		items = append(items, item)
	}

	body := map[string]any{
		"value": items,
	}
	return armrpc_rest.NewOKResponse(body), nil
}
