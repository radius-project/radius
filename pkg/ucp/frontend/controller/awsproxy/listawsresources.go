/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
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
	ucp_aws "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/aws/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

var _ armrpc_controller.Controller = (*ListAWSResources)(nil)

// ListAWSResources is the controller implementation to get/list AWS resources.
type ListAWSResources struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsClients ucp_aws.Clients
}

// NewListAWSResources creates a new ListAWSResources controller with the given options and AWS clients.
func NewListAWSResources(opts armrpc_controller.Options, awsClients ucp_aws.Clients) (armrpc_controller.Controller, error) {
	return &ListAWSResources{
		Operation:  armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.AWSResource]{}),
		awsClients: awsClients,
	}, nil
}

// Run() reads the region from the request, uses the AWS resource type from the context, and lists the
// resources in the region, returning a response with the list of resources.
func (p *ListAWSResources) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	region, errResponse := readRegionFromRequest(req.URL.Path, p.Options().PathBase)
	if errResponse != nil {
		return errResponse, nil
	}

	cloudControlOpts := []func(*cloudcontrol.Options){CloudControlRegionOption(region)}
	// TODO pagination
	response, err := p.awsClients.CloudControl.ListResources(ctx, &cloudcontrol.ListResourcesInput{
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
