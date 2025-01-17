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

package common

import (
	"context"
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_GetResourceTypeDetails(t *testing.T) {
	t.Run("Get Resource Details Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		resourceProvider := v20231001preview.ResourceProviderSummary{
			Name: to.Ptr("Applications.Test"),
			ResourceTypes: map[string]*v20231001preview.ResourceProviderSummaryResourceType{
				"exampleResources": {
					APIVersions: map[string]map[string]any{
						"2023-10-01-preview": {},
					},
				},
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetResourceProviderSummary(gomock.Any(), "local", "Applications.Test").
			Return(resourceProvider, nil).
			Times(1)

		res, err := GetResourceTypeDetails(context.Background(), "Applications.Test", "exampleResources", appManagementClient)
		require.NoError(t, err)
		require.Equal(t, "Applications.Test/exampleResources", res.Name)

	})

	t.Run("Get Resource Details Failure - Resource Provider Not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetResourceProviderSummary(gomock.Any(), "local", "Applications.Test").
			Return(v20231001preview.ResourceProviderSummary{}, radcli.Create404Error()).
			Times(1)

		_, err := GetResourceTypeDetails(context.Background(), "Applications.Test", "exampleResources", appManagementClient)

		require.Error(t, err)
		require.Equal(t, "The resource provider \"Applications.Test\" was not found or has been deleted.", err.Error())
	})

	t.Run("Get Resource Details Failures Other Than Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetResourceProviderSummary(gomock.Any(), "local", "Applications.Test").
			Return(v20231001preview.ResourceProviderSummary{}, errors.New("some error occurred")).
			Times(1)

		_, err := GetResourceTypeDetails(context.Background(), "Applications.Test", "exampleResources", appManagementClient)

		require.Error(t, err)
	})
}
