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

package extraproperties

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kafka"
	"github.com/stretchr/testify/require"
)

func Test_Names(t *testing.T) {
	require.Equal(t, []string{
		"BootstrapBrokerString",
		"BootstrapBrokerStringIpv6",
		"BootstrapBrokerStringPublicSaslIam",
		"BootstrapBrokerStringPublicSaslScram",
		"BootstrapBrokerStringPublicTls",
		"BootstrapBrokerStringSaslIam",
		"BootstrapBrokerStringSaslIamIpv6",
		"BootstrapBrokerStringSaslScram",
		"BootstrapBrokerStringSaslScramIpv6",
		"BootstrapBrokerStringTls",
		"BootstrapBrokerStringTlsIpv6",
		"BootstrapBrokerStringVpcConnectivitySaslIam",
		"BootstrapBrokerStringVpcConnectivitySaslScram",
		"BootstrapBrokerStringVpcConnectivityTls",
	}, Names("AWS::MSK::Cluster"))
	// MSK Serverless supports only IAM access control, and dual-stack networking, but not multi-VPC connectivity.
	require.Equal(t, []string{"BootstrapBrokerStringSaslIam", "BootstrapBrokerStringSaslIamIpv6"}, Names("AWS::MSK::ServerlessCluster"))
	require.Equal(t, Names("AWS::MSK::Cluster"), Names("aws::msk::cluster"))
	require.Nil(t, Names("AWS::S3::Bucket"))
	require.Nil(t, Names(""))
}

func Test_List(t *testing.T) {
	require.Len(t, byLowerType, 2, "only the MSK cluster types have extra properties")
	brokers := reflect.TypeFor[kafka.GetBootstrapBrokersOutput]()
	for typeName, properties := range byLowerType {
		require.NotEmpty(t, properties, typeName)
		for name, p := range properties {
			// UCP reads each value from the GetBootstrapBrokers field with the same name.
			field, ok := brokers.FieldByName(name)
			require.True(t, ok, "%s: %s is not a GetBootstrapBrokers field", typeName, name)
			require.Equal(t, reflect.TypeFor[*string](), field.Type, "%s: %s", typeName, name)
			require.Equal(t, "string", p.Type, "%s: %s", typeName, name)
			require.NotEmpty(t, p.Description, "%s: %s", typeName, name)
		}
	}
}

func Test_MustParse(t *testing.T) {
	require.Panics(t, func() { mustParse([]byte("not json")) })
	require.Equal(t, map[string]map[string]property{"aws::a::b": {"X": {Type: "string"}}}, mustParse([]byte(`{"AWS::A::B": {"X": {"type": "string"}}}`)))
}

func Test_Without(t *testing.T) {
	t.Run("removes the extra properties and leaves the input unchanged", func(t *testing.T) {
		in := map[string]any{"ClusterName": "orders", "BootstrapBrokerStringTls": "b-1:9094", "BootstrapBrokerString": "b-1:9092"}
		out := Without("AWS::MSK::Cluster", in)
		require.Equal(t, map[string]any{"ClusterName": "orders"}, out)
		require.Len(t, in, 3)
	})

	t.Run("removes only the type's own list", func(t *testing.T) {
		in := map[string]any{"ClusterName": "orders", "BootstrapBrokerStringSaslIam": "b-1:9098", "BootstrapBrokerStringTls": "b-1:9094"}
		require.Equal(t, map[string]any{"ClusterName": "orders", "BootstrapBrokerStringTls": "b-1:9094"}, Without("AWS::MSK::ServerlessCluster", in))
	})

	t.Run("returns the input when there is nothing to remove", func(t *testing.T) {
		in := map[string]any{"ClusterName": "orders"}
		require.Equal(t, reflect.ValueOf(in).Pointer(), reflect.ValueOf(Without("AWS::MSK::Cluster", in)).Pointer())

		other := map[string]any{"BootstrapBrokerString": "kept for a type with no list"}
		require.Equal(t, reflect.ValueOf(other).Pointer(), reflect.ValueOf(Without("AWS::S3::Bucket", other)).Pointer())

		require.Nil(t, Without("AWS::MSK::Cluster", nil))
	})
}
