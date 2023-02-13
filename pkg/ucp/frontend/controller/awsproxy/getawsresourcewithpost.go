// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
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
	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	awserror "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
)

var _ armrpc_controller.Controller = (*GetAWSResourceWithPost)(nil)

// GetAWSResourceWithPost is the controller implementation to get an AWS resource.
type GetAWSResourceWithPost struct {
	ctrl.Operation[*datamodel.AWSResource, datamodel.AWSResource]
}

// NewGetAWSResourceWithPost creates a new GetAWSResourceWithPost.
func NewGetAWSResourceWithPost(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &GetAWSResourceWithPost{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.AWSResource]{},
		),
	}, nil
}

func (p *GetAWSResourceWithPost) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := logr.FromContextOrDiscard(ctx)
	cloudControlClient, cloudFormationClient, resourceType, id, err := ParseAWSRequest(ctx, *p.Options(), req)
	if err != nil {
		return nil, err
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

	describeTypeOutput, err := cloudFormationClient.DescribeType(ctx, &cloudformation.DescribeTypeInput{
		Type:     types.RegistryTypeResource,
		TypeName: aws.String(resourceType),
	})
	if err != nil {
		return awserror.HandleAWSError(err)
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

	logger.Info("Fetching resource", "resourceType", resourceType, "resourceID", awsResourceIdentifier)
	response, err := cloudControlClient.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &resourceType,
		Identifier: aws.String(awsResourceIdentifier),
	})
	if awsclient.IsAWSResourceNotFound(err) {
		return armrpc_rest.NewNotFoundMessageResponse(constructNotFoundResponseMessage(middleware.GetRelativePath(p.BasePath(), req.URL.Path), awsResourceIdentifier)), nil
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

	computedResourceID := computeResourceID(id, awsResourceIdentifier)
	body := map[string]any{
		"id":         computedResourceID,
		"name":       response.ResourceDescription.Identifier,
		"type":       id.Type(),
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
