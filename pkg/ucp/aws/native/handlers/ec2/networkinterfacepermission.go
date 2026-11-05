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

package ec2

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const networkInterfacePermissionType = "AWS::EC2::NetworkInterfacePermission"

var (
	_ native.Handler = (*NetworkInterfacePermission)(nil)
	_ native.Finder  = (*NetworkInterfacePermission)(nil)
)

// permissionNotFound are the codes AWS returns for a permission ID it doesn't have.
var permissionNotFound = []string{"InvalidPermissionId.NotFound", "InvalidNetworkInterfacePermissionId.NotFound", "InvalidNetworkInterfacePermissionID.NotFound"}

// permissionStates are the states a create waits for.
var permissionStates = awsapi.States{Done: []string{string(types.NetworkInterfacePermissionStateCodeGranted)}}

// NetworkInterfacePermission serves AWS::EC2::NetworkInterfacePermission, which lets another account attach a network
// interface to its instances or associate an Elastic IP address with it. Its identifier, Id, is the permission ID,
// such as eni-perm-0123456789abcdef0, which AWS assigns. CreateNetworkInterfacePermission takes no idempotency token,
// so UCP finds a lost create by the network interface, account, and permission. A revoked permission that AWS still
// lists counts as gone.
type NetworkInterfacePermission struct {
	api API
}

// NewNetworkInterfacePermission returns the handler for network interface permissions.
func NewNetworkInterfacePermission(api API) *NetworkInterfacePermission {
	return &NetworkInterfacePermission{api: api}
}

// TypeName returns AWS::EC2::NetworkInterfacePermission.
func (h *NetworkInterfacePermission) TypeName() string {
	return networkInterfacePermissionType
}

// Options returns how UCP uses the handler.
func (h *NetworkInterfacePermission) Options() native.Options {
	return native.Options{
		List:     native.ListRegion,
		Recovery: native.RecoverBySearch,
	}
}

// revoked reports whether AWS has revoked the permission. AWS lists a revoked permission for a while.
func revoked(p types.NetworkInterfacePermission) bool {
	return p.PermissionState != nil && p.PermissionState.State == types.NetworkInterfacePermissionStateCodeRevoked
}

// describe returns the permission with the identifier, or ErrNotFound when AWS has none or revoked it.
func (h *NetworkInterfacePermission) describe(ctx context.Context, region string, identifier string) (*types.NetworkInterfacePermission, error) {
	out, err := h.api.DescribeNetworkInterfacePermissions(ctx, &awsec2.DescribeNetworkInterfacePermissionsInput{
		NetworkInterfacePermissionIds: []string{identifier},
	}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, permissionNotFound...)
	}
	for _, p := range out.NetworkInterfacePermissions {
		if aws.ToString(p.NetworkInterfacePermissionId) == identifier && !revoked(p) {
			return &p, nil
		}
	}
	return nil, native.ErrNotFound
}

func permissionProperties(p *types.NetworkInterfacePermission) (map[string]any, error) {
	return awsapi.EncodeObject(map[string]any{
		"Id":                 p.NetworkInterfacePermissionId,
		"AwsAccountId":       p.AwsAccountId,
		"NetworkInterfaceId": p.NetworkInterfaceId,
		"Permission":         string(p.Permission),
	})
}

// Read returns the permission's properties.
func (h *NetworkInterfacePermission) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	p, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return permissionProperties(p)
}

// Create grants the permission and returns its ID.
func (h *NetworkInterfacePermission) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	p := request.Properties
	out, err := h.api.CreateNetworkInterfacePermission(ctx, &awsec2.CreateNetworkInterfacePermissionInput{
		NetworkInterfaceId: aws.String(awsapi.String(p, "NetworkInterfaceId")),
		AwsAccountId:       aws.String(awsapi.String(p, "AwsAccountId")),
		Permission:         types.InterfacePermissionType(awsapi.String(p, "Permission")),
	}, inRegion(request.Region))
	if err != nil {
		return "", err
	}
	if out.InterfacePermission == nil {
		return "", fmt.Errorf("AWS returned no permission ID for the create of an AWS resource of type %s", networkInterfacePermissionType)
	}
	return aws.ToString(out.InterfacePermission.NetworkInterfacePermissionId), nil
}

// Find returns the ID of the permission that is not revoked for the network interface, account, and permission of
// request, or "" when AWS has none. With more than one, UCP can't tell which one its create made, so it returns an
// error.
func (h *NetworkInterfacePermission) Find(ctx context.Context, request native.CreateRequest) (string, error) {
	p := request.Properties
	input := &awsec2.DescribeNetworkInterfacePermissionsInput{Filters: []types.Filter{
		filter("network-interface-permission.network-interface-id", awsapi.String(p, "NetworkInterfaceId")),
		filter("network-interface-permission.aws-account-id", awsapi.String(p, "AwsAccountId")),
		filter("network-interface-permission.permission", awsapi.String(p, "Permission")),
	}}
	found := []string{}
	pages := awsec2.NewDescribeNetworkInterfacePermissionsPaginator(h.api, input)
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, inRegion(request.Region))
		if err != nil {
			return "", err
		}
		for _, permission := range page.NetworkInterfacePermissions {
			if !revoked(permission) {
				found = append(found, aws.ToString(permission.NetworkInterfacePermissionId))
			}
		}
	}
	switch len(found) {
	case 0:
		return "", nil
	case 1:
		return found[0], nil
	}
	return "", awsapi.Ambiguous("network interface permissions", found)
}

// Update does nothing: every property of a network interface permission is create-only.
func (h *NetworkInterfacePermission) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete revokes the permission. A permission AWS is already revoking had a delete reach AWS before, so the delete
// succeeds.
func (h *NetworkInterfacePermission) Delete(ctx context.Context, region string, identifier string) error {
	p, err := h.describe(ctx, region, identifier)
	if err != nil {
		return err
	}
	if p.PermissionState != nil && p.PermissionState.State == types.NetworkInterfacePermissionStateCodeRevoking {
		return nil
	}
	_, err = h.api.DeleteNetworkInterfacePermission(ctx, &awsec2.DeleteNetworkInterfacePermissionInput{
		NetworkInterfacePermissionId: aws.String(identifier),
	}, inRegion(region))
	return awsapi.NotFound(err, permissionNotFound...)
}

// List returns every permission in the region that is not revoked.
func (h *NetworkInterfacePermission) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	pages := awsec2.NewDescribeNetworkInterfacePermissionsPaginator(h.api, &awsec2.DescribeNetworkInterfacePermissionsInput{})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, inRegion(region))
		if err != nil {
			return nil, err
		}
		for _, p := range page.NetworkInterfacePermissions {
			if revoked(p) {
				continue
			}
			properties, err := permissionProperties(&p)
			if err != nil {
				return nil, err
			}
			out = append(out, native.Resource{Identifier: aws.ToString(p.NetworkInterfacePermissionId), Properties: properties})
		}
	}
	return out, nil
}

// Progress reads the permission's state. A create waits for granted, and a delete for the permission to be revoked.
func (h *NetworkInterfacePermission) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	p, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	state := ""
	if p.PermissionState != nil {
		state = string(p.PermissionState.State)
	}
	if op == native.OperationDelete {
		return native.Progress{State: state, Waiting: string(types.NetworkInterfacePermissionStateCodeRevoked)}, nil
	}
	return permissionStates.Progress(state), nil
}
