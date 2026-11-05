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

package lakeformation

import (
	"context"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	resourceType = "AWS::LakeFormation::Resource"

	// serviceLinkedRolePath is in the ARN of the service-linked role Lake Formation uses for a resource registered with
	// UseServiceLinkedRole, AWSServiceRoleForLakeFormationDataAccess.
	serviceLinkedRolePath = ":role/aws-service-role/lakeformation.amazonaws.com/"
)

var (
	_ native.Handler           = (*Resource)(nil)
	_ native.IdentifierEncoder = (*Resource)(nil)
)

// Resource serves AWS::LakeFormation::Resource, a data location registered with Lake Formation. Its identifier, Id,
// is ResourceArn.
//
// UseServiceLinkedRole is write-only, because AWS returns only the role, as RoleArn. An update that turns it on for a
// resource registered with another role registers the resource again, because only RegisterResource sets up the
// service-linked role. Every other change goes through UpdateResource.
type Resource struct {
	api API
}

// NewResource returns the handler for Lake Formation registered resources.
func NewResource(api API) *Resource {
	return &Resource{api: api}
}

// TypeName returns AWS::LakeFormation::Resource.
func (h *Resource) TypeName() string {
	return resourceType
}

// Options returns how UCP uses the handler.
func (h *Resource) Options() native.Options {
	return native.Options{List: native.ListRegion, Recovery: native.RecoverByIdentifier}
}

// Identifier returns ResourceArn, or "" when it holds "/", such as an S3 location with a path. Cloud Control serves
// the type in some regions with the raw ResourceArn as its identifier, so it can't be escaped to one resource ID segment.
func (h *Resource) Identifier(properties map[string]any) (string, error) {
	arn := awsapi.String(properties, "ResourceArn")
	if strings.Contains(arn, "/") {
		return "", nil
	}
	return arn, nil
}

func (h *Resource) describe(ctx context.Context, region string, identifier string) (*types.ResourceInfo, error) {
	out, err := h.api.DescribeResource(ctx, &lakeformation.DescribeResourceInput{ResourceArn: aws.String(identifier)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, notFoundCode)
	}
	if out.ResourceInfo == nil {
		return nil, native.ErrNotFound
	}
	return out.ResourceInfo, nil
}

// Read returns the registered resource's properties.
func (h *Resource) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	info, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return resourceProperties(identifier, info), nil
}

// resourceProperties returns the properties of the registered location arn. DescribeResource doesn't return the ARN
// in ResourceInfo, so it comes from the identifier.
func resourceProperties(arn string, info *types.ResourceInfo) map[string]any {
	properties := map[string]any{"Id": arn, "ResourceArn": arn}
	if info.RoleArn != nil {
		properties["RoleArn"] = *info.RoleArn
	}
	if info.HybridAccessEnabled != nil {
		properties["HybridAccessEnabled"] = *info.HybridAccessEnabled
	}
	if info.WithFederation != nil {
		properties["WithFederation"] = *info.WithFederation
	}
	return properties
}

// isServiceLinkedRole reports whether the role ARN is Lake Formation's service-linked role.
func isServiceLinkedRole(roleArn string) bool {
	return strings.Contains(roleArn, serviceLinkedRolePath)
}

// Create registers the resource.
func (h *Resource) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	if err := h.register(ctx, request.Region, request.Properties); err != nil {
		return "", err
	}
	return awsapi.String(request.Properties, "ResourceArn"), nil
}

func (h *Resource) register(ctx context.Context, region string, properties map[string]any) error {
	input := &lakeformation.RegisterResourceInput{}
	if err := awsapi.Decode(awsapi.Without(properties, "Id"), input); err != nil {
		return err
	}
	_, err := h.api.RegisterResource(ctx, input, inRegion(region))
	return err
}

// updatable are the properties UpdateResource changes.
var updatable = []string{"RoleArn", "HybridAccessEnabled", "WithFederation"}

// Update registers the resource again when the template turns UseServiceLinkedRole on for a resource registered with
// another role. Otherwise it changes the role, HybridAccessEnabled, and WithFederation with UpdateResource. A boolean
// the template removed goes back to false. UpdateResource needs a role, so a template that leaves out RoleArn, or uses
// the service-linked role, keeps the role AWS has.
func (h *Resource) Update(ctx context.Context, request native.UpdateRequest) error {
	info, err := h.describe(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	changed := awsapi.Changed(request)
	useServiceLinkedRole, _ := request.Desired["UseServiceLinkedRole"].(bool)
	if slices.Contains(changed, "UseServiceLinkedRole") && useServiceLinkedRole && !isServiceLinkedRole(aws.ToString(info.RoleArn)) {
		return awsapi.Calls(
			func() error {
				_, err := h.api.DeregisterResource(ctx, &lakeformation.DeregisterResourceInput{ResourceArn: aws.String(request.Identifier)}, inRegion(request.Region))
				return err
			},
			func() error { return h.register(ctx, request.Region, request.Desired) },
		)
	}
	if !slices.ContainsFunc(changed, func(name string) bool { return slices.Contains(updatable, name) }) {
		return nil
	}
	input := &lakeformation.UpdateResourceInput{ResourceArn: aws.String(request.Identifier), RoleArn: info.RoleArn}
	if role := awsapi.String(request.Desired, "RoleArn"); role != "" && !useServiceLinkedRole {
		input.RoleArn = aws.String(role)
	}
	if slices.Contains(changed, "HybridAccessEnabled") {
		value, _ := request.Desired["HybridAccessEnabled"].(bool)
		input.HybridAccessEnabled = aws.Bool(value)
	}
	if slices.Contains(changed, "WithFederation") {
		value, _ := request.Desired["WithFederation"].(bool)
		input.WithFederation = aws.Bool(value)
	}
	_, err = h.api.UpdateResource(ctx, input, inRegion(request.Region))
	return err
}

// Delete deregisters the resource.
func (h *Resource) Delete(ctx context.Context, region string, identifier string) error {
	_, err := h.api.DeregisterResource(ctx, &lakeformation.DeregisterResourceInput{ResourceArn: aws.String(identifier)}, inRegion(region))
	return awsapi.NotFound(err, notFoundCode)
}

// List returns every resource registered with Lake Formation in the region.
func (h *Resource) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	var token *string
	for {
		page, err := h.api.ListResources(ctx, &lakeformation.ListResourcesInput{NextToken: token}, inRegion(region))
		if err != nil {
			return nil, err
		}
		for i := range page.ResourceInfoList {
			info := &page.ResourceInfoList[i]
			out = append(out, native.Resource{Identifier: aws.ToString(info.ResourceArn), Properties: resourceProperties(aws.ToString(info.ResourceArn), info)})
		}
		if token = page.NextToken; token == nil {
			return out, nil
		}
	}
}

// Progress reports every operation as finished, because registering, updating, and deregistering take effect at once.
func (h *Resource) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	return native.Progress{Done: true}, nil
}
