// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

import (
	"github.com/aws/aws-sdk-go/service/cloudformation"
)

//go:generate mockgen -destination=./mock_awscloudformationclient.go -package=aws -self_package github.com/project-radius/radius/pkg/ucp/aws github.com/project-radius/radius/pkg/ucp/aws AWSCloudFormationClient
type AWSCloudFormationClient interface {
	DescribeType(*cloudformation.DescribeTypeInput) (*cloudformation.DescribeTypeOutput, error)
}

var _ = AWSCloudFormationClient(&cloudformation.CloudFormation{})
