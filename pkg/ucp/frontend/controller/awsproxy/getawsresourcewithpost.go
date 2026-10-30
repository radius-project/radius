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

	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/middleware"
	ucpaws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/servicecontext"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/ucplog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	"github.com/radius-project/radius/pkg/retry"
)

var _ armrpc_controller.Controller = (*GetAWSResourceWithPost)(nil)

// GetAWSResourceWithPost is the controller implementation to get an AWS resource.
type GetAWSResourceWithPost struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsClients ucpaws.Clients
	retryer    *retry.Retryer
}

// NewGetAWSResourceWithPost creates a new GetAWSResourceWithPost controller with the given options and AWS clients.
func NewGetAWSResourceWithPost(opts armrpc_controller.Options, awsClients ucpaws.Clients, retryer *retry.Retryer) (armrpc_controller.Controller, error) {
	return &GetAWSResourceWithPost{
		Operation:  armrpc_controller.NewOperation(opts, armrpc_controller.ResourceOptions[datamodel.AWSResource]{}),
		awsClients: awsClients,
		retryer:    retryer,
	}, nil
}

// Run reads an AWS resource that Radius may not have created, for a Bicep `existing` block that sets name or
// properties. It reads AWS by the top-level name in the body when it is set, and otherwise by the identifier in the
// properties. It computes the resource ID and returns an OK response with the resource details. If the resource is not found,
// it returns a NotFound response. If any other error occurs, it returns an error response.
func (p *GetAWSResourceWithPost) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	serviceCtx := servicecontext.AWSRequestContextFromContext(ctx)
	region, errResponse := readRegionFromRequest(req.URL.Path, p.Options().PathBase)
	if errResponse != nil {
		return errResponse, nil
	}
	if errResponse := unsupportedTypeResponse(serviceCtx.ResourceTypeInAWSFormat()); errResponse != nil {
		return errResponse, nil
	}

	body, err := readRequestBody(req)
	if err != nil {
		return badRequest(err.Error()), nil
	}
	name, errResponse := body.nameString()
	if errResponse != nil {
		return errResponse, nil
	}

	cloudFormationOpts := []func(*cloudformation.Options){CloudFormationRegionOption(region)}
	describeTypeOutput, err := p.awsClients.CloudFormation.DescribeType(ctx, &cloudformation.DescribeTypeInput{
		Type:     types.RegistryTypeResource,
		TypeName: new(serviceCtx.ResourceTypeInAWSFormat()),
	}, cloudFormationOpts...)
	if err != nil {
		return ucpaws.HandleAWSError(ctx, err)
	}

	// A top-level name is the AWS identifier of the resource. Otherwise the identifier comes from the properties.
	awsResourceIdentifier := name
	if awsResourceIdentifier == "" {
		awsResourceIdentifier, err = getPrimaryIdentifierFromMultiIdentifiers(body.Properties, aws.ToString(describeTypeOutput.Schema))
		if err != nil {
			return badRequest(fmt.Sprintf("Set name, or the identifier properties of the AWS resource type %s: %s", serviceCtx.ResourceTypeInAWSFormat(), err.Error())), nil
		}
	}

	cloudcontrolOpts := []func(*cloudcontrol.Options){CloudControlRegionOption(region)}
	logger.Info("Fetching resource", "resourceType", serviceCtx.ResourceTypeInAWSFormat(), "resourceID", awsResourceIdentifier)

	var response *cloudcontrol.GetResourceOutput
	if err := p.retryer.RetryFunc(ctx, func(ctx context.Context) error {
		response, err = p.awsClients.CloudControl.GetResource(ctx, &cloudcontrol.GetResourceInput{
			TypeName:   new(serviceCtx.ResourceTypeInAWSFormat()),
			Identifier: aws.String(awsResourceIdentifier),
		}, cloudcontrolOpts...)

		// If the resource is not found, retry.
		if ucpaws.IsAWSResourceNotFoundError(err) {
			return retry.RetryableError(err)
		}

		// If any other error occurs, return the error.
		return err
	}); err != nil {
		if ucpaws.IsAWSResourceNotFoundError(err) {
			return armrpc_rest.NewNotFoundMessageResponse(constructNotFoundResponseMessage(middleware.GetRelativePath(p.Options().PathBase, req.URL.Path), awsResourceIdentifier)), nil
		} else {
			return ucpaws.HandleAWSError(ctx, err)
		}
	}

	resourceProperties := map[string]any{}
	if response.ResourceDescription.Properties != nil {
		err := json.Unmarshal([]byte(*response.ResourceDescription.Properties), &resourceProperties)
		if err != nil {
			return nil, err
		}
	}

	computedResourceID := computeResourceID(serviceCtx.ResourceID, awsResourceIdentifier)
	responseBody := map[string]any{
		"id":         computedResourceID,
		"name":       response.ResourceDescription.Identifier,
		"type":       serviceCtx.ResourceID.Type(),
		"properties": resourceProperties,
	}
	return armrpc_rest.NewOKResponse(responseBody), nil
}

func constructNotFoundResponseMessage(path string, resourceIDs string) string {
	path = strings.Split(path, "/:")[0]
	resourceIDs = strings.ReplaceAll(resourceIDs, "|", ", ")
	message := fmt.Sprintf("Resource %s with primary identifiers %s not found", path, resourceIDs)
	return message
}
