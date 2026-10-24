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

package cmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/radius-project/radius/pkg/cli/clients"
	generated "github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/output"
)

const (
	sharedBucketARN       = "arn:aws:s3:::shared-bucket"
	sharedBucketBicepID   = "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/shared-bucket"
	sharedBucketTFID      = "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket"
	deletingResourceID    = "/planes/radius/local/resourceGroups/team-a/providers/Radius.Storage/buckets/data"
	otherResourceID       = "/planes/radius/local/resourceGroups/team-b/providers/Radius.Storage/buckets/data"
	secondOtherResourceID = "/planes/radius/local/resourceGroups/team-c/providers/Applications.Core/extenders/bucket"
)

// testOutputResource builds a status.outputResources entry the way the API returns it. An empty
// providerResourceID leaves the field out, like an output resource written before it existed.
func testOutputResource(id string, providerResourceID string, radiusManaged bool) map[string]any {
	entry := map[string]any{"id": id, "radiusManaged": radiusManaged}
	if providerResourceID != "" {
		entry["providerResourceId"] = providerResourceID
	}

	return entry
}

func testResourceWithOutputResources(id string, outputResources ...map[string]any) generated.GenericResource {
	entries := []any{}
	for _, outputResource := range outputResources {
		entries = append(entries, outputResource)
	}

	return generated.GenericResource{
		ID:         new(id),
		Properties: map[string]any{"status": map[string]any{"outputResources": entries}},
	}
}

func Test_FindSharedOutputResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		deleting []generated.GenericResource
		others   []generated.GenericResource
		expected []SharedOutputResources
	}{
		{
			name:     "same provider resource ID under different output resource IDs",
			deleting: []generated.GenericResource{testResourceWithOutputResources(deletingResourceID, testOutputResource(sharedBucketTFID, sharedBucketARN, true))},
			others:   []generated.GenericResource{testResourceWithOutputResources(otherResourceID, testOutputResource(sharedBucketBicepID, sharedBucketARN, true))},
			expected: []SharedOutputResources{{ResourceID: deletingResourceID, OutputResources: []SharedOutputResource{{Name: sharedBucketARN, UsedBy: []string{otherResourceID}}}}},
		},
		{
			name:     "same output resource ID without provider resource IDs",
			deleting: []generated.GenericResource{testResourceWithOutputResources(deletingResourceID, testOutputResource(sharedBucketBicepID, "", true))},
			others:   []generated.GenericResource{testResourceWithOutputResources(otherResourceID, testOutputResource(sharedBucketBicepID, "", false))},
			expected: []SharedOutputResources{{ResourceID: deletingResourceID, OutputResources: []SharedOutputResource{{Name: sharedBucketBicepID, UsedBy: []string{otherResourceID}}}}},
		},
		{
			name:     "same output resource ID when only one side has a provider resource ID",
			deleting: []generated.GenericResource{testResourceWithOutputResources(deletingResourceID, testOutputResource(sharedBucketBicepID, sharedBucketARN, true))},
			others:   []generated.GenericResource{testResourceWithOutputResources(otherResourceID, testOutputResource(sharedBucketBicepID, "", false))},
			expected: []SharedOutputResources{{ResourceID: deletingResourceID, OutputResources: []SharedOutputResource{{Name: sharedBucketARN, UsedBy: []string{otherResourceID}}}}},
		},
		{
			name:     "output resource IDs compare case insensitively",
			deleting: []generated.GenericResource{testResourceWithOutputResources(deletingResourceID, testOutputResource(sharedBucketBicepID, "", true))},
			others:   []generated.GenericResource{testResourceWithOutputResources(otherResourceID, testOutputResource("/planes/aws/aws/accounts/123456789012/regions/global/providers/aws.s3/bucket/shared-bucket", "", false))},
			expected: []SharedOutputResources{{ResourceID: deletingResourceID, OutputResources: []SharedOutputResource{{Name: sharedBucketBicepID, UsedBy: []string{otherResourceID}}}}},
		},
		{
			name:     "a provider resource ID does not match a different output resource ID that has none",
			deleting: []generated.GenericResource{testResourceWithOutputResources(deletingResourceID, testOutputResource(sharedBucketTFID, sharedBucketARN, true))},
			others: []generated.GenericResource{
				testResourceWithOutputResources(otherResourceID, testOutputResource(sharedBucketBicepID, sharedBucketARN, true)),
				testResourceWithOutputResources(secondOtherResourceID, testOutputResource(sharedBucketBicepID, "", false)),
			},
			expected: []SharedOutputResources{{ResourceID: deletingResourceID, OutputResources: []SharedOutputResource{{Name: sharedBucketARN, UsedBy: []string{otherResourceID}}}}},
		},
		{
			name:     "different provider resource IDs do not match",
			deleting: []generated.GenericResource{testResourceWithOutputResources(deletingResourceID, testOutputResource(sharedBucketTFID, sharedBucketARN, true))},
			others:   []generated.GenericResource{testResourceWithOutputResources(otherResourceID, testOutputResource(sharedBucketTFID, "arn:aws:s3:::other-bucket", true))},
			expected: []SharedOutputResources{},
		},
		{
			name:     "different output resource IDs without provider resource IDs do not match",
			deleting: []generated.GenericResource{testResourceWithOutputResources(deletingResourceID, testOutputResource(sharedBucketTFID, "", true))},
			others:   []generated.GenericResource{testResourceWithOutputResources(otherResourceID, testOutputResource(sharedBucketBicepID, "", true))},
			expected: []SharedOutputResources{},
		},
		{
			name:     "output resources Radius does not manage are not deleted, so they are not shared",
			deleting: []generated.GenericResource{testResourceWithOutputResources(deletingResourceID, testOutputResource(sharedBucketTFID, sharedBucketARN, false))},
			others:   []generated.GenericResource{testResourceWithOutputResources(otherResourceID, testOutputResource(sharedBucketBicepID, sharedBucketARN, true))},
			expected: []SharedOutputResources{},
		},
		{
			name: "resources deleted together are not reported",
			deleting: []generated.GenericResource{
				testResourceWithOutputResources(deletingResourceID, testOutputResource(sharedBucketTFID, sharedBucketARN, true)),
				testResourceWithOutputResources(otherResourceID, testOutputResource(sharedBucketBicepID, sharedBucketARN, true)),
			},
			others: []generated.GenericResource{
				testResourceWithOutputResources(deletingResourceID, testOutputResource(sharedBucketTFID, sharedBucketARN, true)),
				testResourceWithOutputResources(otherResourceID, testOutputResource(sharedBucketBicepID, sharedBucketARN, true)),
			},
			expected: []SharedOutputResources{},
		},
		{
			name:     "resources without output resources",
			deleting: []generated.GenericResource{{ID: new(deletingResourceID)}},
			others:   []generated.GenericResource{{ID: new(otherResourceID), Properties: map[string]any{"status": map[string]any{}}}},
			expected: []SharedOutputResources{},
		},
		{
			name: "a repeated output resource is reported once",
			deleting: []generated.GenericResource{testResourceWithOutputResources(deletingResourceID,
				testOutputResource(sharedBucketTFID, sharedBucketARN, true),
				testOutputResource(sharedBucketTFID, sharedBucketARN, true),
			)},
			others:   []generated.GenericResource{testResourceWithOutputResources(otherResourceID, testOutputResource(sharedBucketBicepID, sharedBucketARN, true))},
			expected: []SharedOutputResources{{ResourceID: deletingResourceID, OutputResources: []SharedOutputResource{{Name: sharedBucketARN, UsedBy: []string{otherResourceID}}}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, FindSharedOutputResources(tt.deleting, tt.others))
		})
	}
}

func Test_FindSharedOutputResources_NamesEveryOtherResource(t *testing.T) {
	t.Parallel()

	deleting := []generated.GenericResource{testResourceWithOutputResources(deletingResourceID, testOutputResource(sharedBucketTFID, sharedBucketARN, true))}
	others := []generated.GenericResource{
		testResourceWithOutputResources(otherResourceID, testOutputResource(sharedBucketBicepID, sharedBucketARN, true)),
		testResourceWithOutputResources(secondOtherResourceID, testOutputResource(sharedBucketTFID, sharedBucketARN, false)),
	}

	expected := []SharedOutputResources{{
		ResourceID:      deletingResourceID,
		OutputResources: []SharedOutputResource{{Name: sharedBucketARN, UsedBy: []string{otherResourceID, secondOtherResourceID}}},
	}}
	require.Equal(t, expected, FindSharedOutputResources(deleting, others))
}

func Test_WarnSharedOutputResources(t *testing.T) {
	t.Parallel()

	deleting := []generated.GenericResource{testResourceWithOutputResources(deletingResourceID, testOutputResource(sharedBucketTFID, sharedBucketARN, true))}

	t.Run("logs the shared output resources", func(t *testing.T) {
		t.Parallel()
		client := clients.NewMockApplicationsManagementClient(gomock.NewController(t))
		client.EXPECT().ListResourcesInPlane(gomock.Any()).Return(append([]generated.GenericResource{
			testResourceWithOutputResources(otherResourceID, testOutputResource(sharedBucketBicepID, sharedBucketARN, true)),
		}, deleting...), nil)

		outputSink := &output.MockOutput{}
		WarnSharedOutputResources(t.Context(), client, outputSink, "Radius.Storage/buckets/data", deleting)

		require.Equal(t, []any{
			output.LogOutput{Format: msgSharedOutputResources, Params: []any{"Radius.Storage/buckets/data"}},
			output.LogOutput{Format: msgSharedOutputResource, Params: []any{sharedBucketARN, otherResourceID}},
		}, outputSink.Writes)
	})

	t.Run("logs nothing when nothing is shared", func(t *testing.T) {
		t.Parallel()
		client := clients.NewMockApplicationsManagementClient(gomock.NewController(t))
		client.EXPECT().ListResourcesInPlane(gomock.Any()).Return(deleting, nil)

		outputSink := &output.MockOutput{}
		WarnSharedOutputResources(t.Context(), client, outputSink, "Radius.Storage/buckets/data", deleting)

		require.Empty(t, outputSink.Writes)
	})

	t.Run("skips listing when no output resource is Radius managed", func(t *testing.T) {
		t.Parallel()
		// The mock fails the test on any call, so this also checks that no list call is made.
		client := clients.NewMockApplicationsManagementClient(gomock.NewController(t))

		outputSink := &output.MockOutput{}
		unmanaged := []generated.GenericResource{
			testResourceWithOutputResources(deletingResourceID, testOutputResource(sharedBucketTFID, sharedBucketARN, false)),
			{ID: new(otherResourceID)},
		}
		WarnSharedOutputResources(t.Context(), client, outputSink, "application 'my-app'", unmanaged)

		require.Empty(t, outputSink.Writes)
	})

	t.Run("logs a warning when the check fails", func(t *testing.T) {
		t.Parallel()
		listErr := errors.New("list failed")
		client := clients.NewMockApplicationsManagementClient(gomock.NewController(t))
		client.EXPECT().ListResourcesInPlane(gomock.Any()).Return(nil, listErr)

		outputSink := &output.MockOutput{}
		WarnSharedOutputResources(t.Context(), client, outputSink, "application 'my-app'", deleting)

		require.Equal(t, []any{
			output.LogOutput{Format: msgSharedOutputResourcesError, Params: []any{"application 'my-app'", listErr}},
		}, outputSink.Writes)
	})
}
