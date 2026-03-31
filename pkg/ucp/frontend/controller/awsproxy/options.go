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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"

	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
)

// CloudControlRegionOption sets the region for the CloudControl client.
func CloudControlRegionOption(region string) func(*cloudcontrol.Options) {
	return func(o *cloudcontrol.Options) {
		o.Region = region
	}
}

// CloudFormationRegionOption sets the region for the CloudFormation client.
func CloudFormationRegionOption(region string) func(*cloudformation.Options) {
	return func(o *cloudformation.Options) {
		o.Region = region
	}
}

// cloudControlRoleARN returns a pointer to the IRSA role ARN if configured,
// or nil otherwise. When non-nil, this should be passed to CloudControl API
// calls so CloudFormation assumes the role directly.
func cloudControlRoleARN(ctx context.Context, clients ucp_aws.Clients) *string {
	if clients.CloudControlRoleARN == nil {
		return nil
	}
	arn := clients.CloudControlRoleARN(ctx)
	if arn == "" {
		return nil
	}
	return aws.String(arn)
}
