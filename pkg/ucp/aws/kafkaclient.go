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

	"github.com/aws/aws-sdk-go-v2/service/kafka"
)

// AWSKafkaClient is the part of the Amazon MSK client UCP uses: reading the broker addresses of a cluster, which Cloud
// Control does not return.
//
//go:generate go tool mockgen -typed -destination=./mock_awskafkaclient.go -package=aws -self_package github.com/radius-project/radius/pkg/ucp/aws github.com/radius-project/radius/pkg/ucp/aws AWSKafkaClient
type AWSKafkaClient interface {
	GetBootstrapBrokers(ctx context.Context, params *kafka.GetBootstrapBrokersInput, optFns ...func(*kafka.Options)) (*kafka.GetBootstrapBrokersOutput, error)
}

var _ = AWSKafkaClient(&kafka.Client{})
