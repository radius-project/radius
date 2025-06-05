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
	"testing"

	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_GetResourceTypeDetailsWithUCPClient(t *testing.T) {
	t.Run("Get Resource Details Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNoError)
		require.NoError(t, err)

		res, err := GetResourceTypeDetailsWithUCPClient(context.Background(), "MyCompany.Resources", "testResources", clientFactory)
		require.NoError(t, err)
		require.Equal(t, "MyCompany.Resources/testResources", res.Name)

	})

	t.Run("Get Resource Details Failure - Resource Provider Not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNotFoundError)
		require.NoError(t, err)

		_, err = GetResourceTypeDetailsWithUCPClient(context.Background(), "MyCompany.Resources", "testResources", clientFactory)
		require.Error(t, err)
		require.Equal(t, "The resource provider \"MyCompany.Resources\" was not found or has been deleted.", err.Error())
	})

	t.Run("Get Resource Details Failures Other Than Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerInternalError)
		require.NoError(t, err)

		_, err = GetResourceTypeDetailsWithUCPClient(context.Background(), "MyCompany.Resources", "testResources", clientFactory)
		require.Error(t, err)
	})
}
