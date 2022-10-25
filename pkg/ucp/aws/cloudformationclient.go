// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
)

//go:generate mockgen -destination=./mock_awscloudformationclient.go -package=aws -self_package github.com/project-radius/radius/pkg/ucp/aws github.com/project-radius/radius/pkg/ucp/aws AWSCloudFormationClient
type AWSCloudFormationClient interface {
	DescribeType(ctx context.Context, params *cloudformation.DescribeTypeInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeTypeOutput, error)
}

var _ = AWSCloudFormationClient(&cloudformation.Client{})
