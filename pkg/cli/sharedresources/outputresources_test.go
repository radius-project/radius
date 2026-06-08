/*
Copyright 2026 The Radius Authors.

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

package sharedresources

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestOutputResourcesFromGenericResource(t *testing.T) {
	resource := generated.GenericResource{
		Properties: map[string]any{
			"status": map[string]any{
				"outputResources": []any{
					map[string]any{
						"id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/my-bucket",
						"additionalProperties": map[string]any{
							rpv1.OutputResourceProviderResourceIDProperty:     "arn:aws:s3:::my-bucket",
							rpv1.OutputResourceProviderResourceIDKindProperty: rpv1.OutputResourceProviderResourceIDKindAWSARN,
						},
					},
				},
			},
		},
	}

	outputResources := OutputResourcesFromGenericResource(resource)

	require.Len(t, outputResources, 1)
	require.Equal(t, "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/my-bucket", outputResources[0].ID.String())
	require.Equal(t, "arn:aws:s3:::my-bucket", outputResources[0].AdditionalProperties[rpv1.OutputResourceProviderResourceIDProperty])
	require.Equal(t, rpv1.OutputResourceProviderResourceIDKindAWSARN, outputResources[0].AdditionalProperties[rpv1.OutputResourceProviderResourceIDKindProperty])
}

func TestFindSharedReferences_MatchesProviderResourceID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	targetID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Datastores/redisCaches/target"
	sharedID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Datastores/redisCaches/shared"
	resourceType := "Applications.Datastores/redisCaches"

	target := makeResource(targetID, "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/my-bucket", "arn:aws:s3:::my-bucket")
	shared := makeResource(sharedID, "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/my-bucket", "arn:aws:s3:::my-bucket")

	client := clients.NewMockApplicationsManagementClient(ctrl)
	client.EXPECT().
		ListAllResourceTypesNames(gomock.Any(), "local").
		Return([]string{resourceType}, nil).
		Times(1)
	client.EXPECT().
		ListResourcesOfType(gomock.Any(), resourceType).
		Return([]generated.GenericResource{target, shared}, nil).
		Times(1)

	references, err := FindSharedReferences(context.Background(), client, target, map[string]bool{targetID: true})

	require.NoError(t, err)
	require.Equal(t, []SharedReference{{
		ResourceID:       sharedID,
		OutputResourceID: "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/my-bucket",
	}}, references)
}

func makeResource(resourceID string, outputResourceID string, providerResourceID string) generated.GenericResource {
	return generated.GenericResource{
		ID: &resourceID,
		Properties: map[string]any{
			"status": map[string]any{
				"outputResources": []any{
					map[string]any{
						"id": outputResourceID,
						"additionalProperties": map[string]any{
							rpv1.OutputResourceProviderResourceIDProperty:     providerResourceID,
							rpv1.OutputResourceProviderResourceIDKindProperty: rpv1.OutputResourceProviderResourceIDKindAWSARN,
						},
					},
				},
			},
		},
	}
}
