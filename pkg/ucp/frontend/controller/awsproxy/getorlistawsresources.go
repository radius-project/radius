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
	awstypes "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
)

var _ ctrl.Controller = (*GetOrListAWSResource)(nil)

// GetOrListAWSResource is the controller implementation to get/list AWS resources.
type GetOrListAWSResource struct {
	ctrl.BaseController
}

// NewGetOrListAWSResource creates a new GetOrListAWSResource.
func NewGetOrListAWSResource(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetOrListAWSResource{ctrl.NewBaseController(opts)}, nil
}

func (p *GetOrListAWSResource) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	resourceType := ctx.Value(AWSResourceTypeKey).(string)
	client := ctx.Value(AWSClientKey).(awstypes.AWSClient)
	id := ctx.Value(AWSResourceID).(resources.ID)

	if id.IsCollection() {
		return p.listAWSResources(ctx, resourceType, client, id)
	} else {
		return p.getAWSResource(ctx, resourceType, client, id)
	}
}

func (p *GetOrListAWSResource) getAWSResource(ctx context.Context, resourceType string, client awstypes.AWSClient, id resources.ID) (rest.Response, error) {
	response, err := client.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &resourceType,
		Identifier: aws.String(id.Name()),
	})
	if awstypes.IsAWSResourceNotFound(err) {
		return rest.NewNotFoundResponse(id.String()), nil
	} else if err != nil {
		return awstypes.HandleAWSError(err)
	}

	properties := map[string]interface{}{}
	if response.ResourceDescription.Properties != nil {
		err := json.Unmarshal([]byte(*response.ResourceDescription.Properties), &properties)
		if err != nil {
			return nil, err
		}
	}

	body := map[string]interface{}{
		"id":         id.String(),
		"name":       response.ResourceDescription.Identifier,
		"type":       id.Type(),
		"properties": properties,
	}
	return rest.NewOKResponse(body), nil
}

func (p *GetOrListAWSResource) listAWSResources(ctx context.Context, resourceType string, client awstypes.AWSClient, id resources.ID) (rest.Response, error) {
	// TODO pagination
	response, err := client.ListResources(ctx, &cloudcontrol.ListResourcesInput{
		TypeName: &resourceType,
	})
	if err != nil {
		return awstypes.HandleAWSError(err)
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

		individualId := id.ChangeName(*result.Identifier)

		item := map[string]interface{}{
			"id":         individualId.String(),
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
