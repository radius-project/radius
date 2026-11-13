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

package api

import (
	"encoding/json"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/test/testutil"

	"github.com/stretchr/testify/require"
)

func TestDelegatingResourcesEnvelope(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		supplied   bool
		invalid    bool
	}{
		{name: "absent", body: `{"properties":{"image":"unchanged"}}`},
		{name: "empty", body: `{"properties":{"image":"unchanged"},"delegatingResources":[]}`, supplied: true},
		{name: "engine transport", body: `{"properties":{"image":"unchanged","__radiusDelegatingResources":[]}}`, supplied: true},
		{name: "engine transport null", body: `{"properties":{"__radiusDelegatingResources":null}}`, invalid: true},
		{name: "two envelopes", body: `{"properties":{"__radiusDelegatingResources":[]},"delegatingResources":[]}`, invalid: true},
		{name: "null", body: `{"delegatingResources": null}`, invalid: true},
		{name: "object", body: `{"delegatingResources":{}}`, invalid: true},
		{name: "nested", body: `{"delegatingResources":[{"delegatingResources":[]}]}`, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var versioned DynamicResource
			err := json.Unmarshal([]byte(tc.body), &versioned)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			converted, err := versioned.ConvertTo()
			require.NoError(t, err)
			resource := converted.(*datamodel.DynamicResource)
			require.Equal(t, tc.supplied, resource.DelegatingResources != nil)
			require.Equal(t, map[string]any{"image": "unchanged"}, resource.Properties)
			stored, err := json.Marshal(resource)
			require.NoError(t, err)
			require.NotContains(t, string(stored), `"delegatingResources"`)
			require.NotContains(t, string(stored), `"__radiusDelegatingResources"`)
			require.NoError(t, versioned.ConvertFrom(resource))
			public, err := json.Marshal(&versioned)
			require.NoError(t, err)
			require.NotContains(t, string(public), `"delegatingResources"`)
			require.NotContains(t, string(public), `"__radiusDelegatingResources"`)
		})
	}
}

func Test_DynamicResource_ConvertVersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *datamodel.DynamicResource
		err      error
	}{
		{
			filename: "dynamicresource-resource.json",
			expected: &datamodel.DynamicResource{
				ID:       "/planes/radius/local/resourceGroups/test/providers/Applications.Test/testResources/testResource",
				Name:     "testResource",
				Type:     "Applications.Test/testResources",
				Location: "global",
				Tags: map[string]string{
					"env": "dev",
				},
				UpdatedAPIVersion: "2025-08-01-preview",
				Properties: map[string]any{
					"message": "Hello, world!",
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &DynamicResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			r.SetAPIVersion("2025-08-01-preview")
			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				ct := dm.(*datamodel.DynamicResource)
				require.Equal(t, tt.expected, ct)
			}
		})
	}
}

func Test_DynamicResource_ConvertDataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *DynamicResource
		err      error
	}{
		{
			filename: "dynamicresource-datamodel.json",
			expected: &DynamicResource{
				ID:       new("/planes/radius/local/resourceGroups/test/providers/Applications.Test/testResources/testResource"),
				Name:     new("testResource"),
				Type:     new("Applications.Test/testResources"),
				Location: new("global"),
				Tags: map[string]*string{
					"env": new("dev"),
				},
				Properties: map[string]any{
					"provisioningState": fromProvisioningStateDataModel(v1.ProvisioningStateSucceeded),
					"message":           "Hello, world!",
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			dm := &datamodel.DynamicResource{}
			err := json.Unmarshal(rawPayload, dm)
			require.NoError(t, err)

			resource := &DynamicResource{}
			err = resource.ConvertFrom(dm)

			// Avoid hardcoding the SystemData field in tests.
			tt.expected.SystemData = fromSystemDataDataModel(dm.SystemData)

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, resource)
			}
		})
	}
}

func Test_DynamicResource_ConvertDataModelToVersioned_NilProperties(t *testing.T) {
	dm := &datamodel.DynamicResource{}
	resource := &DynamicResource{}

	err := resource.ConvertFrom(dm)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"provisioningState": fromProvisioningStateDataModel(dm.AsyncProvisioningState),
	}, resource.Properties)
}

// Radius.* resources keep output resources in their properties, so providerResourceId reaches the API unchanged.
func Test_DynamicResource_ConvertDataModelToVersioned_ProviderResourceID(t *testing.T) {
	outputResources := []any{
		map[string]any{
			"id":                 "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/my-bucket",
			"radiusManaged":      true,
			"providerResourceId": "arn:aws:s3:::my-bucket",
		},
	}
	dm := &datamodel.DynamicResource{}
	dm.Properties = map[string]any{"status": map[string]any{"outputResources": outputResources}}

	resource := &DynamicResource{}
	err := resource.ConvertFrom(dm)
	require.NoError(t, err)

	b, err := json.Marshal(resource)
	require.NoError(t, err)
	versioned := map[string]any{}
	require.NoError(t, json.Unmarshal(b, &versioned))
	status := versioned["properties"].(map[string]any)["status"].(map[string]any)
	require.Equal(t, "arn:aws:s3:::my-bucket", status["outputResources"].([]any)[0].(map[string]any)["providerResourceId"])
}
