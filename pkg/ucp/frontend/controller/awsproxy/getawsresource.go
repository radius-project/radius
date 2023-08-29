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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/to"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	ucp_aws "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/aws/servicecontext"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

var _ armrpc_controller.Controller = (*GetAWSResource)(nil)

// GetAWSResource is the controller implementation to get AWS resource.
type GetAWSResource struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsClients ucp_aws.Clients
}

// NewGetAWSResource creates a new GetAWSResource controller with the given options and AWS clients, and returns it or an
// error if one occurs.
func NewGetAWSResource(opts armrpc_controller.Options, awsClients ucp_aws.Clients) (armrpc_controller.Controller, error) {
	return &GetAWSResource{
		Operation:  armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.AWSResource]{}),
		awsClients: awsClients,
	}, nil
}

// Run() reads the region from the request, gets the resource from AWS using the region and resource type and
// ID, and returns a response containing the resource's properties.
func (p *GetAWSResource) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	region, errResponse := readRegionFromRequest(req.URL.Path, p.Options().PathBase)
	if errResponse != nil {
		return errResponse, nil
	}

	cloudControlOpts := []func(*cloudcontrol.Options){CloudControlRegionOption(region)}
	response, err := p.awsClients.CloudControl.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
		Identifier: aws.String(serviceCtx.ResourceID.Name()),
	}, cloudControlOpts...)
	if awsclient.IsAWSResourceNotFoundError(err) {
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
