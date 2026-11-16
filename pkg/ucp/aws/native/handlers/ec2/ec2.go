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

// Package ec2 has the native handlers for the Amazon EC2 types Cloud Control can't create in some or all regions:
// Client VPN endpoints and their target network associations, network ACL entries, network interface permissions,
// and route propagation from a virtual private gateway.
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

// API is the part of the EC2 client the handlers use.
type API interface {
	CreateClientVpnEndpoint(ctx context.Context, params *awsec2.CreateClientVpnEndpointInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateClientVpnEndpointOutput, error)
	DescribeClientVpnEndpoints(ctx context.Context, params *awsec2.DescribeClientVpnEndpointsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeClientVpnEndpointsOutput, error)
	ModifyClientVpnEndpoint(ctx context.Context, params *awsec2.ModifyClientVpnEndpointInput, optFns ...func(*awsec2.Options)) (*awsec2.ModifyClientVpnEndpointOutput, error)
	DeleteClientVpnEndpoint(ctx context.Context, params *awsec2.DeleteClientVpnEndpointInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteClientVpnEndpointOutput, error)
	GetClientVpnEndpointAuthorizationPolicy(ctx context.Context, params *awsec2.GetClientVpnEndpointAuthorizationPolicyInput, optFns ...func(*awsec2.Options)) (*awsec2.GetClientVpnEndpointAuthorizationPolicyOutput, error)
	ModifyClientVpnEndpointAuthorizationPolicy(ctx context.Context, params *awsec2.ModifyClientVpnEndpointAuthorizationPolicyInput, optFns ...func(*awsec2.Options)) (*awsec2.ModifyClientVpnEndpointAuthorizationPolicyOutput, error)
	DeleteClientVpnEndpointAuthorizationPolicy(ctx context.Context, params *awsec2.DeleteClientVpnEndpointAuthorizationPolicyInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteClientVpnEndpointAuthorizationPolicyOutput, error)

	AssociateClientVpnTargetNetwork(ctx context.Context, params *awsec2.AssociateClientVpnTargetNetworkInput, optFns ...func(*awsec2.Options)) (*awsec2.AssociateClientVpnTargetNetworkOutput, error)
	DescribeClientVpnTargetNetworks(ctx context.Context, params *awsec2.DescribeClientVpnTargetNetworksInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeClientVpnTargetNetworksOutput, error)
	DisassociateClientVpnTargetNetwork(ctx context.Context, params *awsec2.DisassociateClientVpnTargetNetworkInput, optFns ...func(*awsec2.Options)) (*awsec2.DisassociateClientVpnTargetNetworkOutput, error)

	CreateNetworkAclEntry(ctx context.Context, params *awsec2.CreateNetworkAclEntryInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateNetworkAclEntryOutput, error)
	ReplaceNetworkAclEntry(ctx context.Context, params *awsec2.ReplaceNetworkAclEntryInput, optFns ...func(*awsec2.Options)) (*awsec2.ReplaceNetworkAclEntryOutput, error)
	DeleteNetworkAclEntry(ctx context.Context, params *awsec2.DeleteNetworkAclEntryInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteNetworkAclEntryOutput, error)
	DescribeNetworkAcls(ctx context.Context, params *awsec2.DescribeNetworkAclsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeNetworkAclsOutput, error)

	CreateNetworkInterfacePermission(ctx context.Context, params *awsec2.CreateNetworkInterfacePermissionInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateNetworkInterfacePermissionOutput, error)
	DescribeNetworkInterfacePermissions(ctx context.Context, params *awsec2.DescribeNetworkInterfacePermissionsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeNetworkInterfacePermissionsOutput, error)
	DeleteNetworkInterfacePermission(ctx context.Context, params *awsec2.DeleteNetworkInterfacePermissionInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteNetworkInterfacePermissionOutput, error)

	EnableVgwRoutePropagation(ctx context.Context, params *awsec2.EnableVgwRoutePropagationInput, optFns ...func(*awsec2.Options)) (*awsec2.EnableVgwRoutePropagationOutput, error)
	DisableVgwRoutePropagation(ctx context.Context, params *awsec2.DisableVgwRoutePropagationInput, optFns ...func(*awsec2.Options)) (*awsec2.DisableVgwRoutePropagationOutput, error)
	DescribeRouteTables(ctx context.Context, params *awsec2.DescribeRouteTablesInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeRouteTablesOutput, error)
}

var inRegion = awsapi.InRegion[awsec2.Options]

// filter returns an EC2 filter that matches any of values.
func filter(name string, values ...string) types.Filter {
	return types.Filter{Name: aws.String(name), Values: values}
}

// invalidIdentifier returns the error for an identifier that doesn't have the parts of the type's identifier.
func invalidIdentifier(typeName string, identifier string, format string) error {
	return &native.Failure{
		Message:     fmt.Sprintf("the identifier %q of the AWS type %s is not of the form %s", identifier, typeName, format),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}
