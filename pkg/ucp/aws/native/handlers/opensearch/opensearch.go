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

// Package opensearch has the native handlers for Amazon OpenSearch Service resources that have no CloudFormation
// type: managed interface VPC endpoints, and the accounts and services a domain lets create them.
package opensearch

import (
	"context"
	"errors"
	"fmt"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"

	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

// API is the part of the Amazon OpenSearch Service client the handlers use.
type API interface {
	CreateVpcEndpoint(ctx context.Context, params *opensearch.CreateVpcEndpointInput, optFns ...func(*opensearch.Options)) (*opensearch.CreateVpcEndpointOutput, error)
	DescribeVpcEndpoints(ctx context.Context, params *opensearch.DescribeVpcEndpointsInput, optFns ...func(*opensearch.Options)) (*opensearch.DescribeVpcEndpointsOutput, error)
	UpdateVpcEndpoint(ctx context.Context, params *opensearch.UpdateVpcEndpointInput, optFns ...func(*opensearch.Options)) (*opensearch.UpdateVpcEndpointOutput, error)
	DeleteVpcEndpoint(ctx context.Context, params *opensearch.DeleteVpcEndpointInput, optFns ...func(*opensearch.Options)) (*opensearch.DeleteVpcEndpointOutput, error)
	ListVpcEndpoints(ctx context.Context, params *opensearch.ListVpcEndpointsInput, optFns ...func(*opensearch.Options)) (*opensearch.ListVpcEndpointsOutput, error)
	AuthorizeVpcEndpointAccess(ctx context.Context, params *opensearch.AuthorizeVpcEndpointAccessInput, optFns ...func(*opensearch.Options)) (*opensearch.AuthorizeVpcEndpointAccessOutput, error)
	RevokeVpcEndpointAccess(ctx context.Context, params *opensearch.RevokeVpcEndpointAccessInput, optFns ...func(*opensearch.Options)) (*opensearch.RevokeVpcEndpointAccessOutput, error)
	ListVpcEndpointAccess(ctx context.Context, params *opensearch.ListVpcEndpointAccessInput, optFns ...func(*opensearch.Options)) (*opensearch.ListVpcEndpointAccessOutput, error)
}

// EC2API is the part of the Amazon EC2 client the VPC endpoint handler uses, to find a VPC's default security group.
type EC2API interface {
	DescribeSecurityGroups(ctx context.Context, params *awsec2.DescribeSecurityGroupsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeSecurityGroupsOutput, error)
}

var (
	inRegion    = awsapi.InRegion[opensearch.Options]
	ec2InRegion = awsapi.InRegion[awsec2.Options]
)

// invalid returns the client failure of a template AWS would reject, before any AWS call.
func invalid(typeName string, format string, args ...any) error {
	return &native.Failure{
		Message:     fmt.Sprintf("%s: %s", typeName, fmt.Sprintf(format, args...)),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// beforeChange returns err, from a call made before any call that changes AWS, as a failure AWS applied none of, so UCP
// drops the pending values instead of counting them as possibly applied. ErrNotFound and a native.Failure stay as
// they are.
func beforeChange(err error) error {
	err = awsapi.Rejected(err)
	var failure *native.Failure
	if err == nil || errors.Is(err, native.ErrNotFound) || errors.As(err, &failure) {
		return err
	}
	return &native.Failure{Message: err.Error(), ClientError: ucp_aws.IsClientFault(err), Outcome: native.OutcomeRejected}
}

// stringList returns the list of strings at name in properties, or nil when it is absent. ok is false when the value
// isn't a list of strings, so it is never sent as a shorter list.
func stringList(properties map[string]any, name string) (list []string, ok bool) {
	value, present := properties[name]
	if !present || value == nil {
		return nil, true
	}
	items, isList := value.([]any)
	if !isList {
		return nil, false
	}
	list = []string{}
	for _, item := range items {
		s, isString := item.(string)
		if !isString {
			return nil, false
		}
		list = append(list, s)
	}
	return list, true
}
