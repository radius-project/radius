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
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*GetAWSResourceWithPost)(nil)

// GetAWSResourceWithPost is the controller implementation to get an AWS resource.
type GetAWSResourceWithPost struct {
	ctrl.BaseController
}

// NewGetAWSResourceWithPost creates a new GetAWSResourceWithPost.
func NewGetAWSResourceWithPost(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &GetAWSResourceWithPost{ctrl.NewBaseController(opts)}, nil
}

func (p *GetAWSResourceWithPost) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.GetLogger(ctx)
	cloudControlClient, cloudFormationClient, resourceType, id, err := ParseAWSRequest(ctx, p.Options, req)
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

	awsResourceIdentifier, err := getResourceIDWithMultiIdentifiers(ctx, cloudFormationClient, req.URL.Path, resourceType, properties)
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
		return armrpc_rest.NewNotFoundMessageResponse(constructNotFoundResponseMessage(p.GetRelativePath(req.URL.Path), awsResourceIdentifier)), nil
	} else if err != nil {
		return awsclient.HandleAWSError(err)
	}

	resourceProperties := map[string]interface{}{}
	if response.ResourceDescription.Properties != nil {
		err := json.Unmarshal([]byte(*response.ResourceDescription.Properties), &resourceProperties)
		if err != nil {
			return nil, err
		}
	}

	computedResourceID := computeResourceID(id, awsResourceIdentifier)
	body := map[string]interface{}{
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
