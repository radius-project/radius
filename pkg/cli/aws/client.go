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
package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

//go:generate mockgen -typed -destination=./client_mock.go -package=aws -self_package github.com/radius-project/radius/pkg/cli/aws github.com/radius-project/radius/pkg/cli/aws Client

// Client is an interface that abstracts `rad init`'s interactions with AWS. This is for testing purposes. This is only exported because mockgen requires it.
type Client interface {
	// GetCallerIdentity gets information about the provided credentials.
	GetCallerIdentity(ctx context.Context, region string) (*sts.GetCallerIdentityOutput, error)
	// ListRegions lists the AWS regions available (fetched from EC2.DescribeRegions API).
	ListRegions(ctx context.Context, region string) (*ec2.DescribeRegionsOutput, error)
}

// NewClient returns a new Client.
func NewClient() Client {
	return &client{}
}

type client struct{}

var _ Client = &client{}

// GetCallerIdentity gets information about the provided credentials.
func (c *client) GetCallerIdentity(ctx context.Context, region string) (*sts.GetCallerIdentityOutput, error) {
	// Load the AWS SDK config and credentials
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Create an STS client
	svc := sts.NewFromConfig(cfg)

	// Call GetCallerIdentity
	result, err := svc.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ListRegions lists the AWS regions available (fetched from EC2.DescribeRegions API).
func (c *client) ListRegions(ctx context.Context, region string) (*ec2.DescribeRegionsOutput, error) {
	// Load the AWS SDK config and credentials
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Create an EC2 client
	svc := ec2.NewFromConfig(cfg)

	// Call DescribeRegions
	input := &ec2.DescribeRegionsInput{}
	result, err := svc.DescribeRegions(ctx, input)
	if err != nil {
		return nil, err
	}

	return result, nil
}
