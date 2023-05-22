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
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/to"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/aws/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
)

var _ armrpc_controller.Controller = (*ListAWSResources)(nil)

// ListAWSResources is the controller implementation to get/list AWS resources.
type ListAWSResources struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsOptions ctrl.AWSOptions
	basePath   string
}

// NewListAWSResources creates a new ListAWSResources.
func NewListAWSResources(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &ListAWSResources{
		Operation: armrpc_controller.NewOperation(opts.Options,
			armrpc_controller.ResourceOptions[datamodel.AWSResource]{},
		),
		awsOptions: opts.AWSOptions,
		basePath:   opts.BasePath,
	}, nil
}

func (p *ListAWSResources) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	region, errResponse := readRegionFromRequest(req.URL.Path, p.basePath)
	if errResponse != nil {
		return errResponse, nil
	}

	cloudControlOpts := []func(*cloudcontrol.Options){CloudControlRegionOption(region)}
	// TODO pagination
	response, err := p.awsOptions.AWSCloudControlClient.ListResources(ctx, &cloudcontrol.ListResourcesInput{
		TypeName: to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
	}, cloudControlOpts...)
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
			"id":         path.Join(serviceCtx.ResourceID.String(), resourceName),
			"name":       result.Identifier,
			"type":       serviceCtx.ResourceID.Type(),
			"properties": properties,
		}
		items = append(items, item)
	}

	body := map[string]any{
		"value": items,
	}
	return armrpc_rest.NewOKResponse(body), nil
}
