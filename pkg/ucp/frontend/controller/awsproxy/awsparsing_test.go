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
	"github.com/wI2L/jsondiff"
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

func Test_GeneratePatch(t *testing.T) {
	testCases := []struct {
		name          string
		currentState  map[string]interface{}
		desiredState  map[string]interface{}
		schema        map[string]interface{}
		expectedPatch jsondiff.Patch
	}{
		{
			"No updates creates empty patch",
			map[string]interface{}{
				"A": "B",
				"C": map[string]interface{}{
					"D": map[string]interface{}{
						"E": "F",
					},
					"G": map[string]interface{}{
						"I": "J",
					},
					"K": "L",
				},
			},
			map[string]interface{}{
				"A": "B",
				"C": map[string]interface{}{
					"G": map[string]interface{}{
						"I": "J",
					},
				},
			},
			map[string]interface{}{
				"properties": map[string]interface{}{
					"A": map[string]interface{}{},
					"C": map[string]interface{}{},
				},
				"readOnlyProperties": []interface{}{
					"/properties/C/D/E",
				},
				"createOnlyProperties": []interface{}{
					"/properties/C/K",
				},
			},
			nil,
		},
		{
			"Update creates patch",
			map[string]interface{}{
				"A": "B",
				"C": map[string]interface{}{
					"D": map[string]interface{}{
						"E": "F",
					},
					"G": map[string]interface{}{
						"I": "J",
					},
					"K": "L",
				},
			},
			map[string]interface{}{
				"A": "Test",
				"C": map[string]interface{}{
					"G": map[string]interface{}{
						"I": "Test2",
					},
				},
			},
			map[string]interface{}{
				"properties": map[string]interface{}{
					"A": map[string]interface{}{},
					"C": map[string]interface{}{},
				},
				"readOnlyProperties": []interface{}{
					"/properties/C/D/E",
				},
				"createOnlyProperties": []interface{}{
					"/properties/C/K",
				},
			},
			jsondiff.Patch{
				{
					Type:     "replace",
					Path:     "/A",
					OldValue: "B",
					Value:    "Test",
				},
				{
					Type:     "replace",
					Path:     "/C/G/I",
					OldValue: "J",
					Value:    "Test2",
				},
			},
		},
		{
			"Specify create-only properties",
			map[string]interface{}{
				"A": map[string]interface{}{
					"B": map[string]interface{}{
						"C": "D",
						"E": "F",
					},
				},
			},
			map[string]interface{}{
				"A": map[string]interface{}{
					"B": map[string]interface{}{
						"C": "D",
						"E": "Test",
					},
				},
			},
			map[string]interface{}{
				"properties": map[string]interface{}{
					"A": map[string]interface{}{},
				},
				"createOnlyProperties": []interface{}{
					"/properties/A/B/C",
				},
			},
			jsondiff.Patch{
				{
					Type:     "replace",
					Path:     "/A/B/E",
					OldValue: "F",
					Value:    "Test",
				},
			},
		},
		{
			"Remove object",
			map[string]interface{}{
				"A": map[string]interface{}{
					"B": map[string]interface{}{
						"C": "D",
						"E": "F",
					},
				},
				"G": "H",
			},
			map[string]interface{}{
				"G": "H",
			},
			map[string]interface{}{
				"properties": map[string]interface{}{
					"A": map[string]interface{}{},
					"G": map[string]interface{}{},
				},
			},
			jsondiff.Patch{
				{
					Type: "remove",
					Path: "/A",
					OldValue: map[string]interface{}{
						"B": map[string]interface{}{
							"C": "D",
							"E": "F",
						},
					},
					Value: nil,
				},
			},
		},
		{
			"Updating create-and-write-only property noops",
			map[string]interface{}{
				"A": "B",
			},
			map[string]interface{}{
				"A": "C",
			},
			map[string]interface{}{
				"properties": map[string]interface{}{
					"A": map[string]interface{}{},
				},
				"createOnlyProperties": []interface{}{
					"/properties/A",
				},
				"writeOnlyProperties": []interface{}{
					"/properties/A",
				},
			},
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			desiredStateBytes, err := json.Marshal(testCase.desiredState)
			require.NoError(t, err)

			currentStateBytes, err := json.Marshal(testCase.currentState)
			require.NoError(t, err)

			schemaBytes, err := json.Marshal(testCase.schema)
			require.NoError(t, err)

			patch, err := generatePatch(currentStateBytes, desiredStateBytes, schemaBytes)
			require.NoError(t, err)

			require.Equal(t, testCase.expectedPatch, patch)
		})

	}
}
