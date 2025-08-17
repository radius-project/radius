/*
Copyright 2025 The Radius Authors.

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

package delete

import (
	"context"
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_DeleteApplicationWithProgress_ErrorScenarios(t *testing.T) {
	t.Run("Error: Resource ID Parsing Fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		invalidResourceID := "invalid-resource-id"
		appManagementClient.EXPECT().
			ListResourcesInApplication(gomock.Any(), "test-app").
			Return([]generated.GenericResource{
				{
					ID: &invalidResourceID,
				},
			}, nil).
			Times(1)

		options := clients.DeleteOptions{
			ApplicationNameOrID: "test-app",
			ProgressText:        "Deleting application...",
		}

		deleted, err := DeleteApplicationWithProgress(context.Background(), appManagementClient, options)
		require.Error(t, err)
		require.False(t, deleted)
		require.Contains(t, err.Error(), "not a valid resource id")
	})

	t.Run("Error: Application ID Parsing Fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		appManagementClient.EXPECT().
			ListResourcesInApplication(gomock.Any(), "test-app").
			Return([]generated.GenericResource{}, nil).
			Times(1)

		invalidAppID := "invalid-app-id"
		appManagementClient.EXPECT().
			GetApplication(gomock.Any(), "test-app").
			Return(corerp.ApplicationResource{
				ID: &invalidAppID,
				Properties: &corerp.ApplicationProperties{
					Environment: to.Ptr("/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/test"),
				},
			}, nil).
			Times(1)

		options := clients.DeleteOptions{
			ApplicationNameOrID: "test-app",
			ProgressText:        "Deleting application...",
		}

		deleted, err := DeleteApplicationWithProgress(context.Background(), appManagementClient, options)
		require.Error(t, err)
		require.False(t, deleted)
		require.Contains(t, err.Error(), "not a valid resource id")
	})

	t.Run("Error: Resource ID Parsing Fails After Deletion", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		validResourceID := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/test-container"
		appManagementClient.EXPECT().
			ListResourcesInApplication(gomock.Any(), "test-app").
			Return([]generated.GenericResource{
				{
					ID: &validResourceID,
				},
			}, nil).
			Times(1)

		appManagementClient.EXPECT().
			GetApplication(gomock.Any(), "test-app").
			Return(corerp.ApplicationResource{}, fmt.Errorf("not found")).
			Times(1)

		appManagementClient.EXPECT().
			DeleteApplication(gomock.Any(), "test-app").
			Return(true, nil).
			Times(1)

		options := clients.DeleteOptions{
			ApplicationNameOrID: "test-app",
			ProgressText:        "Deleting application...",
		}

		deleted, err := DeleteApplicationWithProgress(context.Background(), appManagementClient, options)
		require.NoError(t, err)
		require.True(t, deleted)
	})

	t.Run("Error: List Resources Fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		appManagementClient.EXPECT().
			ListResourcesInApplication(gomock.Any(), "test-app").
			Return(nil, fmt.Errorf("failed to list resources")).
			Times(1)

		options := clients.DeleteOptions{
			ApplicationNameOrID: "test-app",
			ProgressText:        "Deleting application...",
		}

		deleted, err := DeleteApplicationWithProgress(context.Background(), appManagementClient, options)
		require.Error(t, err)
		require.False(t, deleted)
		require.Contains(t, err.Error(), "failed to list resources")
	})

	t.Run("Success: Progress Channel Handles Multiple Duplicate Resources", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		resourceID := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/test-container"
		appManagementClient.EXPECT().
			ListResourcesInApplication(gomock.Any(), "test-app").
			Return([]generated.GenericResource{
				{ID: &resourceID},
				{ID: &resourceID},
				{ID: &resourceID},
			}, nil).
			Times(1)

		appManagementClient.EXPECT().
			GetApplication(gomock.Any(), "test-app").
			Return(corerp.ApplicationResource{}, fmt.Errorf("not found")).
			Times(1)

		appManagementClient.EXPECT().
			DeleteApplication(gomock.Any(), "test-app").
			Return(true, nil).
			Times(1)

		options := clients.DeleteOptions{
			ApplicationNameOrID: "test-app",
			ProgressText:        "Deleting application...",
		}

		deleted, err := DeleteApplicationWithProgress(context.Background(), appManagementClient, options)
		require.NoError(t, err)
		require.True(t, deleted)
	})

	t.Run("Success: Handles Nil Resource IDs Gracefully", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		validResourceID := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/test-container"
		appManagementClient.EXPECT().
			ListResourcesInApplication(gomock.Any(), "test-app").
			Return([]generated.GenericResource{
				{ID: nil},
				{ID: &validResourceID},
				{ID: nil},
			}, nil).
			Times(1)

		appManagementClient.EXPECT().
			GetApplication(gomock.Any(), "test-app").
			Return(corerp.ApplicationResource{}, fmt.Errorf("not found")).
			Times(1)

		appManagementClient.EXPECT().
			DeleteApplication(gomock.Any(), "test-app").
			Return(true, nil).
			Times(1)

		options := clients.DeleteOptions{
			ApplicationNameOrID: "test-app",
			ProgressText:        "Deleting application...",
		}

		deleted, err := DeleteApplicationWithProgress(context.Background(), appManagementClient, options)
		require.NoError(t, err)
		require.True(t, deleted)
	})
}
