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
