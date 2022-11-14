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

	actualPrimaryIdentifiers, err := lookupPrimaryIdentifiersForResourceType(ctx, mockClient, "AWS::NetworkManager::Device")
	require.NoError(t, err)

	resourceID, err := getResourceIDFromPrimaryIdentifiers(actualPrimaryIdentifiers, map[string]interface{}{
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

	actualPrimaryIdentifiers, err := lookupPrimaryIdentifiersForResourceType(ctx, mockClient, "AWS::NetworkManager::Device")
	require.NoError(t, err)

	_, err = getResourceIDFromPrimaryIdentifiers(actualPrimaryIdentifiers, map[string]interface{}{
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
