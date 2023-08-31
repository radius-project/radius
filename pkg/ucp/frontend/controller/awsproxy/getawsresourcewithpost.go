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
	"fmt"
	http "net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/middleware"
	"github.com/radius-project/radius/pkg/to"
	awsclient "github.com/radius-project/radius/pkg/ucp/aws"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/servicecontext"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*GetAWSResourceWithPost)(nil)

// GetAWSResourceWithPost is the controller implementation to get an AWS resource.
type GetAWSResourceWithPost struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsClients ucp_aws.Clients
}

// NewGetAWSResourceWithPost creates a new GetAWSResourceWithPost controller with the given options and AWS clients.
func NewGetAWSResourceWithPost(opts armrpc_controller.Options, awsClients ucp_aws.Clients) (armrpc_controller.Controller, error) {
	return &GetAWSResourceWithPost{
		Operation:  armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.AWSResource]{}),
		awsClients: awsClients,
	}, nil
}

// Run() reads the region from the request, reads properties from the body, fetches the resource
// from AWS, computes the resource ID and returns an OK response with the resource details. If the resource is not found,
// it returns a NotFound response. If any other error occurs, it returns an error response.
func (p *GetAWSResourceWithPost) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	region, errResponse := readRegionFromRequest(req.URL.Path, p.Options().PathBase)
	if errResponse != nil {
		return errResponse, nil
	}

	properties, err := readPropertiesFromBody(req)
	if err != nil {
		e := v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInvalid,
				Message: err.Error(),
			},
		}

		return armrpc_rest.NewBadRequestARMResponse(e), nil
	}

	cloudFormationOpts := []func(*cloudformation.Options){CloudFormationWithRegionOption(region)}
	describeTypeOutput, err := p.awsClients.CloudFormation.DescribeType(ctx, &cloudformation.DescribeTypeInput{
		Type:     types.RegistryTypeResource,
		TypeName: to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
	}, cloudFormationOpts...)
	if err != nil {
		return ucp_aws.HandleAWSError(err)
	}

	awsResourceIdentifier, err := getPrimaryIdentifierFromMultiIdentifiers(ctx, properties, *describeTypeOutput.Schema)
	if err != nil {
		e := v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    v1.CodeInvalid,
				Message: err.Error(),
			},
		}

		return armrpc_rest.NewBadRequestARMResponse(e), nil
	}

	cloudcontrolOpts := []func(*cloudcontrol.Options){CloudControlRegionOption(region)}
	logger.Info("Fetching resource", "resourceType", serviceCtx.ResourceTypeInAWSFormat(), "resourceID", awsResourceIdentifier)
	response, err := p.awsClients.CloudControl.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   to.Ptr(serviceCtx.ResourceTypeInAWSFormat()),
		Identifier: aws.String(awsResourceIdentifier),
	}, cloudcontrolOpts...)

	if awsclient.IsAWSResourceNotFoundError(err) {
		return armrpc_rest.NewNotFoundMessageResponse(constructNotFoundResponseMessage(middleware.GetRelativePath(p.Options().PathBase, req.URL.Path), awsResourceIdentifier)), nil
	} else if err != nil {
		return awsclient.HandleAWSError(err)
	}

	resourceProperties := map[string]any{}
	if response.ResourceDescription.Properties != nil {
		err := json.Unmarshal([]byte(*response.ResourceDescription.Properties), &resourceProperties)
		if err != nil {
			return nil, err
		}
	}

	computedResourceID := computeResourceID(serviceCtx.ResourceID, awsResourceIdentifier)
	body := map[string]any{
		"id":         computedResourceID,
		"name":       response.ResourceDescription.Identifier,
		"type":       serviceCtx.ResourceID.Type(),
		"properties": resourceProperties,
	}
	return armrpc_rest.NewOKResponse(body), nil
}

func constructNotFoundResponseMessage(path string, resourceIDs string) string {
	path = strings.Split(path, "/:")[0]
	resourceIDs = strings.ReplaceAll(resourceIDs, "|", ", ")
	message := fmt.Sprintf("Resource %s with primary identifiers %s not found", path, resourceIDs)
	return message
}
