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

package awstypes

import (
	"bufio"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// nativeGapTypes are the types Cloud Control cannot create that are neither legacy nor deprecated. They are out of
// scope for this package: UCP reports that Cloud Control cannot create them.
var nativeGapTypes = []string{
	"AWS::AppSync::GraphQLSchema",
	"AWS::CloudWatch::AnomalyDetector",
	"AWS::DAX::Cluster",
	"AWS::DMS::ReplicationInstance",
	"AWS::DocDB::DBCluster",
	"AWS::DocDB::DBInstance",
	"AWS::EC2::ClientVpnEndpoint",
	"AWS::EC2::ClientVpnTargetNetworkAssociation",
	"AWS::EC2::NetworkAclEntry",
	"AWS::EC2::NetworkInterfacePermission",
	"AWS::EC2::VPNGatewayRoutePropagation",
	"AWS::EMR::Cluster",
	"AWS::ElasticLoadBalancingV2::ListenerCertificate",
	"AWS::FSx::FileSystem",
	"AWS::IAM::AccessKey",
	"AWS::KinesisAnalyticsV2::ApplicationCloudWatchLoggingOption",
	"AWS::KinesisAnalyticsV2::ApplicationOutput",
	"AWS::KinesisAnalyticsV2::ApplicationReferenceDataSource",
	"AWS::LakeFormation::DataLakeSettings",
	"AWS::LakeFormation::Permissions",
	"AWS::LakeFormation::Resource",
	"AWS::ManagedBlockchain::Member",
	"AWS::MediaLive::Channel",
	"AWS::SageMaker::ExperimentTrialComponent",
	"AWS::ServiceCatalog::AcceptedPortfolioShare",
	"Alexa::ASK::Skill",
}

// readSnapshot returns the handlers of every NON_PROVISIONABLE type in the snapshot the generator reads.
func readSnapshot(t *testing.T) map[string][]string {
	f, err := os.Open("non_provisionable_types.txt")
	require.NoError(t, err)
	defer f.Close()

	out := map[string][]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		out[fields[0]] = strings.Split(fields[1], ",")
	}
	require.NoError(t, scanner.Err())
	return out
}

// Test_EveryNonProvisionableTypeIsClassified checks that each type Cloud Control cannot create is exactly one of
// read-only, legacy, deprecated, or a native gap.
func Test_EveryNonProvisionableTypeIsClassified(t *testing.T) {
	counts := map[string]int{}
	for typeName := range readSnapshot(t) {
		_, legacy := Replacement(typeName)
		_, deprecated := DeprecatedReason(typeName)
		classes := []string{}
		for class, ok := range map[string]bool{
			"read-only":  IsReadOnly(typeName),
			"legacy":     legacy,
			"deprecated": deprecated,
			"native gap": slices.Contains(nativeGapTypes, typeName),
		} {
			if ok {
				classes = append(classes, class)
			}
		}
		require.Len(t, classes, 1, "%s is classified as %v", typeName, classes)
		counts[classes[0]]++
	}
	require.Equal(t, len(ReadOnlyTypes()), counts["read-only"])
	require.Equal(t, 7, counts["legacy"])
	require.Equal(t, 89, counts["deprecated"])
	require.Equal(t, len(nativeGapTypes), counts["native gap"])
}

// Test_UnsupportedTypesCannotBeUsedThroughCloudControl checks that every legacy and deprecated type is
// NON_PROVISIONABLE and has no read handler. UCP rejects these types on every operation, so this guarantees it never
// blocks a type that Cloud Control can create or read.
func Test_UnsupportedTypesCannotBeUsedThroughCloudControl(t *testing.T) {
	snapshot := readSnapshot(t)
	unsupported := slices.Collect(func(yield func(string) bool) {
		for typeName := range legacyTypes {
			if !yield(typeName) {
				return
			}
		}
		for _, group := range deprecatedTypes {
			for _, typeName := range group.types {
				if !yield(typeName) {
					return
				}
			}
		}
	})
	require.Len(t, unsupported, 96)
	for _, typeName := range unsupported {
		handlers, ok := snapshot[typeName]
		require.True(t, ok, "%s is not NON_PROVISIONABLE", typeName)
		require.NotContains(t, handlers, "read", typeName)
		require.NotContains(t, handlers, "list", typeName)
	}
}

func Test_UnsupportedMessage(t *testing.T) {
	tests := []struct {
		typeName string
		want     string
	}{
		{"AWS::IAM::Policy", "Radius does not support the legacy AWS resource type AWS::IAM::Policy. Use AWS::IAM::ManagedPolicy, or AWS::IAM::RolePolicy, AWS::IAM::UserPolicy, or AWS::IAM::GroupPolicy for an inline policy instead."},
		{"aws::sqs::queuepolicy", "Radius does not support the legacy AWS resource type aws::sqs::queuepolicy. Use AWS::SQS::QueueInlinePolicy instead."},
		{"AWS::OpsWorks::Stack", "Radius does not support the AWS resource type AWS::OpsWorks::Stack: AWS OpsWorks reached end of life."},
		{"AWS::CloudFormation::WaitCondition", "Radius does not support the AWS resource type AWS::CloudFormation::WaitCondition: it is a CloudFormation template feature, not an AWS resource."},
	}
	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			got, ok := UnsupportedMessage(tt.typeName)
			require.True(t, ok)
			require.Equal(t, tt.want, got)
		})
	}

	// These services have deprecated types, but Cloud Control manages these types.
	for _, typeName := range []string{"AWS::S3::Bucket", "AWS::CloudFormation::Stack", "AWS::Pinpoint::InAppTemplate", "AWS::QLDB::Stream", "AWS::DocDB::DBCluster"} {
		_, ok := UnsupportedMessage(typeName)
		require.False(t, ok, typeName)
	}
}

func Test_IsReadOnly(t *testing.T) {
	require.True(t, IsReadOnly("AWS::EC2::Image"))
	require.True(t, IsReadOnly("aws::ec2::image"))
	require.False(t, IsReadOnly("AWS::S3::Bucket"))
	require.False(t, IsReadOnly("AWS::IAM::Policy"))
	require.True(t, slices.IsSorted(ReadOnlyTypes()))

	types := ReadOnlyTypes()
	types[0] = "changed"
	require.NotEqual(t, "changed", ReadOnlyTypes()[0], "ReadOnlyTypes returns a copy")

	require.Equal(t, "The AWS resource type AWS::EC2::Image is created by AWS, so Radius cannot create or change it. Reference an existing resource with the Bicep `existing` keyword instead.", ReadOnlyMessage("AWS::EC2::Image"))
}
