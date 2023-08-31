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

package awsproxy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func TestGetPrimaryIdentifierFromMultiIdentifiers(t *testing.T) {
	ctx := context.Background()

	schemaObject := map[string]any{
		"primaryIdentifier": []any{
			"/properties/GlobalNetworkId",
			"/properties/DeviceId",
		},
	}

	schemaBytes, err := json.Marshal(schemaObject)
	require.NoError(t, err)

	schema := string(schemaBytes)

	properties := map[string]any{
		"GlobalNetworkId": "global-network-id",
		"DeviceId":        "device-id",
	}

	resourceID, err := getPrimaryIdentifierFromMultiIdentifiers(ctx, properties, schema)
	require.NoError(t, err)
	require.Equal(t, "global-network-id|device-id", resourceID)
}

func TestGetPrimaryIdentifierFromMultiIdentifiers_MissingMandatoryParameters(t *testing.T) {
	ctx := context.Background()

	schemaObject := map[string]any{
		"primaryIdentifier": []any{
			"/properties/GlobalNetworkId",
			"/properties/DeviceId",
		},
	}

	schemaBytes, err := json.Marshal(schemaObject)
	require.NoError(t, err)

	schema := string(schemaBytes)

	properties := map[string]any{
		"GlobalNetworkId": "global-network-id",
	}

	resourceID, err := getPrimaryIdentifierFromMultiIdentifiers(ctx, properties, schema)
	require.Equal(t, resourceID, "")
	require.Error(t, err)
	require.EqualError(t, err, "mandatory property DeviceId is missing")
}

func TestComputeResourceID(t *testing.T) {
	url := "/apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/841861948707/regions/us-west-2/providers/AWS.NetworkManager/Device/:put"
	id, err := resources.Parse(url)
	require.NoError(t, err)
	resourceID := "global-network-id|device-id"
	computedID := computeResourceID(id, resourceID)
	require.Equal(t, "/apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/841861948707/regions/us-west-2/providers/AWS.NetworkManager/Device/global-network-id|device-id", computedID)
}

func TestGetPrimaryIdentifiersFromSchema(t *testing.T) {
	ctx := context.Background()

	schemaObject := map[string]any{
		"primaryIdentifier": []any{
			"/properties/GlobalNetworkId",
			"/properties/DeviceId",
		},
	}

	schemaBytes, err := json.Marshal(schemaObject)
	require.NoError(t, err)

	schema := string(schemaBytes)

	primaryIdentifiers, err := getPrimaryIdentifiersFromSchema(ctx, schema)
	require.NoError(t, err)
	require.Equal(t, []string{"/properties/GlobalNetworkId", "/properties/DeviceId"}, primaryIdentifiers)
}

func TestGetPrimaryIdentifiersFromSchema_PrimaryIdentifierMissing(t *testing.T) {
	ctx := context.Background()

	schemaObject := map[string]any{}

	schemaBytes, err := json.Marshal(schemaObject)
	require.NoError(t, err)

	schema := string(schemaBytes)

	primaryIdentifiers, err := getPrimaryIdentifiersFromSchema(ctx, schema)
	require.Nil(t, primaryIdentifiers)
	require.EqualError(t, err, "primaryIdentifier not found in schema")
}

func TestGetPrimaryIdentifiersFromSchema_PrimaryIdentifierWrongDataType(t *testing.T) {
	ctx := context.Background()

	schemaObject := map[string]any{
		"primaryIdentifier": "/properties/GlobalNetworkId",
	}

	schemaBytes, err := json.Marshal(schemaObject)
	require.NoError(t, err)

	schema := string(schemaBytes)

	primaryIdentifiers, err := getPrimaryIdentifiersFromSchema(ctx, schema)
	require.Nil(t, primaryIdentifiers)
	require.EqualError(t, err, "primaryIdentifier is not an array")
}

func Test_ExtractRegionFromURLPath_Invalid(t *testing.T) {
	URLPath := "/planes/radius/local/resourcegroups/localrp/providers/Microsoft.Resources/deployments/rad-deploy-06221c5e-104d-4472-bb74-876b441c7663"
	region, errResponse := readRegionFromRequest(URLPath, "/apis/api.ucp.dev/v1alpha3")
	require.NotNil(t, errResponse)
	require.Empty(t, region)

}

func Test_ExtractRegionFromURLPath_Valid(t *testing.T) {
	URLPath := "/apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/817312594854/regions/us-west-2/providers/AWS.S3/Bucket/:put?api-version=default"
	region, errResponse := readRegionFromRequest(URLPath, "/apis/api.ucp.dev/v1alpha3")
	require.Nil(t, errResponse)
	require.Equal(t, "us-west-2", region)
}
