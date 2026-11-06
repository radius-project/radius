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

	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	ucpaws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/servicecontext"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
)

var _ armrpc_controller.Controller = (*ListAWSResources)(nil)

// ListAWSResources is the controller implementation to get/list AWS resources.
type ListAWSResources struct {
	armrpc_controller.Operation[*datamodel.AWSResource, datamodel.AWSResource]
	awsClients ucpaws.Clients
}

// NewListAWSResources creates a new ListAWSResources controller with the given options and AWS clients.
func NewListAWSResources(opts armrpc_controller.Options, awsClients ucpaws.Clients) (armrpc_controller.Controller, error) {
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
	if errResponse := unsupportedTypeResponse(serviceCtx.ResourceTypeInAWSFormat()); errResponse != nil {
		return errResponse, nil
	}

	rt, err := routeType(ctx, p.awsClients, serviceCtx.ResourceTypeInAWSFormat(), region)
	if err != nil {
		return ucpaws.HandleAWSError(ctx, err)
	}
	if rt.handler != nil {
		return listNative(ctx, rt.handler, serviceCtx.ResourceID, region, rt.writeOnly)
	}

	cloudControlOpts := []func(*cloudcontrol.Options){CloudControlRegionOption(region)}
	// TODO pagination
	response, err := p.awsClients.CloudControl.ListResources(ctx, &cloudcontrol.ListResourcesInput{
		TypeName: new(serviceCtx.ResourceTypeInAWSFormat()),
	}, cloudControlOpts...)
	if err != nil {
		return ucpaws.HandleAWSError(ctx, err)
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
		removePaths(properties, rt.writeOnly)

		item := map[string]any{
			"id":         serviceCtx.ResourceID.String() + resources.SegmentSeparator + addressSegment(aws.ToString(result.Identifier)),
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

// listNative lists the resources of a type a native handler serves, without the values at the write-only paths. The
// properties go through JSON first, as a read's do, so a nested typed map or slice, such as map[string]string, is
// redacted too. A type that can't be listed, or can be listed only under a parent resource, gets 400: UCP's list has
// no way to name the parent.
func listNative(ctx context.Context, h native.Handler, collectionID resources.ID, region string, writeOnly []string) (armrpc_rest.Response, error) {
	switch h.Options().List {
	case native.ListNone:
		return badRequest(fmt.Sprintf("The AWS resource type %s can't be listed.", h.TypeName())), nil
	case native.ListParent:
		return badRequest(fmt.Sprintf("The AWS resource type %s can be listed only under its parent resource, which Radius does not support.", h.TypeName())), nil
	}

	listed, err := h.List(ctx, region)
	if err != nil {
		return handleError(ctx, collectionID, err)
	}
	items := []any{}
	for _, resource := range listed {
		properties, err := asJSON(resource.Properties)
		if err != nil {
			return nil, err
		}
		removePaths(properties, writeOnly)
		items = append(items, map[string]any{
			"id":         collectionID.String() + resources.SegmentSeparator + addressSegment(resource.Identifier),
			"name":       resource.Identifier,
			"type":       collectionID.Type(),
			"properties": properties,
		})
	}
	return armrpc_rest.NewOKResponse(map[string]any{"value": items}), nil
}
