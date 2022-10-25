// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package awsproxy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/golang/mock/gomock"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func TestResourceIDWithMultiIdentifiers(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockClient := awsclient.NewMockAWSCloudFormationClient(mockCtrl)
	resourceType := "AWS::NetworkManager::Device"

	primaryIdentifiers := map[string]interface{}{
		"primaryIdentifier": []interface{}{
			"/properties/GlobalNetworkId",
			"/properties/DeviceId",
		},
	}
	serialized, err := json.Marshal(primaryIdentifiers)
	require.NoError(t, err)
	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(resourceType),
		Schema:   to.Ptr(string(serialized)),
	}

	input := cloudformation.DescribeTypeInput{
		TypeName: aws.String(resourceType),
		Type:     types.RegistryTypeResource,
	}
	mockClient.EXPECT().DescribeType(ctx, &input).Return(&output, nil)

	url := "http://127.0.0.1:9000/apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/841861948707/regions/us-west-2/providers/AWS.NetworkManager/Device/:put"
	resourceID, err := getResourceIDWithMultiIdentifiers(ctx, mockClient, url, "AWS::NetworkManager::Device", map[string]interface{}{
		"GlobalNetworkId": "global-network-id",
		"DeviceId":        "device-id",
	})
	require.NoError(t, err)
	require.Equal(t, "global-network-id|device-id", resourceID)
}

func TestResourceIDWithMultiIdentifiers_MissingMandatoryParameters(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockClient := awsclient.NewMockAWSCloudFormationClient(mockCtrl)
	resourceType := "AWS::NetworkManager::Device"

	primaryIdentifiers := map[string]interface{}{
		"primaryIdentifier": []interface{}{
			"/properties/GlobalNetworkId",
			"/properties/DeviceId",
		},
	}
	serialized, err := json.Marshal(primaryIdentifiers)
	require.NoError(t, err)
	output := cloudformation.DescribeTypeOutput{
		TypeName: aws.String(resourceType),
		Schema:   to.Ptr(string(serialized)),
	}

	input := cloudformation.DescribeTypeInput{
		TypeName: aws.String(resourceType),
		Type:     types.RegistryTypeResource,
	}
	mockClient.EXPECT().DescribeType(ctx, &input).Return(&output, nil)

	url := "http://127.0.0.1:9000/apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/841861948707/regions/us-west-2/providers/AWS.NetworkManager/Device/:put"
	_, err = getResourceIDWithMultiIdentifiers(ctx, mockClient, url, "AWS::NetworkManager::Device", map[string]interface{}{
		"GlobalNetworkId": "global-network-id",
	})
	require.NotNil(t, err)
	require.Equal(t, "mandatory property DeviceId is missing", err.Error())
}

func TestComputeResourceID(t *testing.T) {
	url := "/apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/841861948707/regions/us-west-2/providers/AWS.NetworkManager/Device/:put"
	id, err := resources.Parse(url)
	require.NoError(t, err)
	resourceID := "global-network-id|device-id"
	computedID := computeResourceID(id, resourceID)
	require.Equal(t, "/apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/841861948707/regions/us-west-2/providers/AWS.NetworkManager/Device/global-network-id|device-id", computedID)
}

func TestFlattenProperties(t *testing.T) {
	properties := map[string]interface{}{
		"A": map[string]interface{}{
			"B": map[string]interface{}{
				"C": "D",
			},
			"E": "F",
		},
		"G": "H",
	}

	flattened := flattenProperties(properties)
	require.Equal(t, map[string]interface{}{
		"A/B/C": "D",
		"A/E":   "F",
		"G":     "H",
	}, flattened)
}

func TestUnflattenProperties(t *testing.T) {
	properties := map[string]interface{}{
		"A/B/C": "D",
		"A/E":   "F",
		"G":     "H",
	}

	unflattened := unflattenProperties(properties)
	require.Equal(t, map[string]interface{}{
		"A": map[string]interface{}{
			"B": map[string]interface{}{
				"C": "D",
			},
			"E": "F",
		},
		"G": "H",
	}, unflattened)
}

func TestFlattenUnflattenInverses(t *testing.T) {
	properties := map[string]interface{}{
		"A": map[string]interface{}{
			"B": map[string]interface{}{
				"C": "D",
			},
			"E": "F",
		},
		"G": "H",
	}

	flattened := flattenProperties(properties)
	unflattened := unflattenProperties(flattened)
	require.Equal(t, properties, unflattened)
}

func TestFlattenUnflattenRealData(t *testing.T) {
	properties := map[string]interface{}{
		"ClusterEndpoint:": map[string]interface{}{
			"Address": "https://A1B2C3D4E5F6.gr7.us-west-2.eks.amazonaws.com",
			"Port":    443,
		},
		"ClusterName": "my-cluster",
	}

	flattened := flattenProperties(properties)
	unflattened := unflattenProperties(flattened)
	require.Equal(t, properties, unflattened)
}

func TestRemovePropertyKeywordFromString(t *testing.T) {
	require.Equal(t, "ClusterEndpoint", removePropertyKeywordFromString("/properties/ClusterEndpoint"))
	require.Equal(t, "ClusterEndpoint/Address", removePropertyKeywordFromString("/properties/ClusterEndpoint/Address"))
	require.Equal(t, "ClusterEndpoint/Address/Port", removePropertyKeywordFromString("/properties/ClusterEndpoint/Address/Port"))
}

func TestMapValues(t *testing.T) {
	var input []string
	var expected []string
	var actual []string

	input = []string{"A", "B", "C"}
	actual = mapValues(input, func(value string) string {
		return value + "test"
	})
	expected = []string{"Atest", "Btest", "Ctest"}
	require.Equal(t, expected, actual)

	input = []string{"/properties/Status", "/properties/ClusterEndpoint/Address", "/properties/ClusterEndpoint/Port", "/properties/Port"}
	actual = mapValues(input, removePropertyKeywordFromString)
	expected = []string{"Status", "ClusterEndpoint/Address", "ClusterEndpoint/Port", "Port"}
	require.Equal(t, expected, actual)
}
