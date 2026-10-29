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

package arnproperties

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Lookup(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		property     string
		known        bool
	}{
		{name: "Arn", resourceType: "AWS::S3::Bucket", property: "Arn", known: true},
		{name: "resource name Arn", resourceType: "AWS::SNS::Topic", property: "TopicArn", known: true},
		{name: "resource name Arn with a different case", resourceType: "AWS::RDS::DBInstance", property: "DBInstanceArn", known: true},
		{name: "read-only primary identifier", resourceType: "AWS::IAM::ManagedPolicy", property: "PolicyArn", known: true},
		{name: "reviewed, two candidates", resourceType: "AWS::OpenSearchService::Domain", property: "Arn", known: true},
		{name: "reviewed, other name", resourceType: "AWS::GameLift::ContainerFleet", property: "FleetArn", known: true},
		{name: "no ARN in the schema", resourceType: "AWS::EC2::VPC", property: "", known: true},
		{name: "only unrelated ARNs", resourceType: "AWS::Redshift::Cluster", property: "", known: true},
		{name: "primary identifier is another resource's ARN", resourceType: "AWS::Lambda::ResourcePolicy", property: "", known: true},
		{name: "case-insensitive type", resourceType: "aws::sns::topic", property: "TopicArn", known: true},
		{name: "unknown type", resourceType: "AWS::Example::Thing", property: "", known: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			property, known := Lookup(tt.resourceType)
			require.Equal(t, tt.property, property)
			require.Equal(t, tt.known, known)
		})
	}
}

func Test_ProviderResourceID(t *testing.T) {
	tests := []struct {
		name string
		arn  string
		want string
	}{
		{name: "log group from Cloud Control", arn: "arn:aws:logs:us-east-2:179022619019:log-group:orders:*", want: "arn:aws:logs:us-east-2:179022619019:log-group:orders"},
		{name: "log group from Terraform", arn: "arn:aws:logs:us-east-2:179022619019:log-group:orders", want: "arn:aws:logs:us-east-2:179022619019:log-group:orders"},
		{name: "log group with a path name", arn: "arn:aws-us-gov:logs:us-gov-west-1:179022619019:log-group:/aws/lambda/orders:*", want: "arn:aws-us-gov:logs:us-gov-west-1:179022619019:log-group:/aws/lambda/orders"},
		{name: "log stream keeps its qualifier", arn: "arn:aws:logs:us-east-2:179022619019:log-group:orders:log-stream:*", want: "arn:aws:logs:us-east-2:179022619019:log-group:orders:log-stream:*"},
		{name: "log group with no name", arn: "arn:aws:logs:us-east-2:179022619019:log-group::*", want: "arn:aws:logs:us-east-2:179022619019:log-group::*"},
		{name: "other logs resource", arn: "arn:aws:logs:us-east-2:179022619019:destination:orders", want: "arn:aws:logs:us-east-2:179022619019:destination:orders"},
		{name: "other service with a qualifier", arn: "arn:aws:lambda:us-east-2:179022619019:function:orders:*", want: "arn:aws:lambda:us-east-2:179022619019:function:orders:*"},
		{name: "not an ARN", arn: "orders:*", want: "orders:*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ProviderResourceID(tt.arn))
		})
	}
}

// Test_ProviderResourceID_LogGroupMatchesAcrossDrivers pins that the Bicep driver, which reads the ARN from Cloud
// Control, and the Terraform driver, which reads it from Terraform state, store the same value for one log group.
func Test_ProviderResourceID_LogGroupMatchesAcrossDrivers(t *testing.T) {
	cloudControlARN := "arn:aws:logs:us-east-2:179022619019:log-group:orders:*"
	terraformARN := "arn:aws:logs:us-east-2:179022619019:log-group:orders"

	require.Equal(t, ProviderResourceID(terraformARN), ProviderResourceID(cloudControlARN))
	require.NotEqual(t, ProviderResourceID(terraformARN), ProviderResourceID("arn:aws:logs:us-east-2:179022619019:log-group:Orders:*"), "log group names are case-sensitive")
}
