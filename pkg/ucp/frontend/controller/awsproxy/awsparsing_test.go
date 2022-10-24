// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package awsproxy

import (
	"encoding/json"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/golang/mock/gomock"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func TestResourceIDWithMultiIdentifiers(t *testing.T) {
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
		Type:     aws.String("RESOURCE"),
	}
	mockClient.EXPECT().DescribeType(&input).Return(&output, nil)

	opts := ctrl.Options{
		AWSCloudFormationClient: mockClient,
	}
	url := "http://127.0.0.1:9000/apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/841861948707/regions/us-west-2/providers/AWS.NetworkManager/Device/:put"
	resourceID, err := getResourceIDWithMultiIdentifiers(opts, url, "AWS::NetworkManager::Device", map[string]interface{}{
		"GlobalNetworkId": "global-network-id",
		"DeviceId":        "device-id",
	})
	require.NoError(t, err)
	require.Equal(t, "global-network-id|device-id", resourceID)
}

func TestResourceIDWithMultiIdentifiers_MissingMandatoryParameters(t *testing.T) {
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
		Type:     aws.String("RESOURCE"),
	}
	mockClient.EXPECT().DescribeType(&input).Return(&output, nil)

	opts := ctrl.Options{
		AWSCloudFormationClient: mockClient,
	}
	url := "http://127.0.0.1:9000/apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/841861948707/regions/us-west-2/providers/AWS.NetworkManager/Device/:put"
	_, err = getResourceIDWithMultiIdentifiers(opts, url, "AWS::NetworkManager::Device", map[string]interface{}{
		"GlobalNetworkId": "global-network-id",
	})
	require.NotNil(t, err)
	require.Equal(t, "Mandatory property DeviceId is missing", err.Error())
}

func TestComputeResourceID(t *testing.T) {
	url := "/apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/841861948707/regions/us-west-2/providers/AWS.NetworkManager/Device/:put"
	id, err := resources.Parse(url)
	require.NoError(t, err)
	resourceID := "global-network-id|device-id"
	computedID := computeResourceID(id, resourceID)
	require.Equal(t, "/apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/841861948707/regions/us-west-2/providers/AWS.NetworkManager/Device/global-network-id|device-id", computedID)
}
