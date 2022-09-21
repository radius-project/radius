// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package awsproxy

import (
	"context"
	"encoding/json"
	http "net/http"

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	awserror "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
)

var _ ctrl.Controller = (*ListAWSResources)(nil)

// ListAWSResources is the controller implementation to get/list AWS resources.
type ListAWSResources struct {
	ctrl.BaseController
}

// NewListAWSResources creates a new ListAWSResources.
func NewListAWSResources(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListAWSResources{ctrl.NewBaseController(opts)}, nil
}

func (p *ListAWSResources) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	client, resourceType, id, err := ParseAWSRequest(ctx, p.Options.BasePath, req)
	if err != nil {
		return nil, err
	}

	// TODO pagination
	response, err := client.ListResources(ctx, &cloudcontrol.ListResourcesInput{
		TypeName: &resourceType,
	})
	if err != nil {
		return awserror.HandleAWSError(err)
	}

	// TODO there some limitations with listing resources:
	//
	// https://docs.aws.amazon.com/cloudcontrolapi/latest/userguide/resource-operations-list.html

	items := []interface{}{}
	for _, result := range response.ResourceDescriptions {
		properties := map[string]interface{}{}
		if result.Properties != nil {
			err := json.Unmarshal([]byte(*result.Properties), &properties)
			if err != nil {
				return nil, err
			}
		}

		item := map[string]interface{}{
			"id":         id.String(),
			"name":       result.Identifier,
			"type":       id.Type(),
			"properties": properties,
		}
		items = append(items, item)
	}

	body := map[string]interface{}{
		"value": items,
	}
	return rest.NewOKResponse(body), nil
}
