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
	"testing"

	"github.com/radius-project/radius/pkg/cli/aws"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_EnterCloudProviderOptions(t *testing.T) {
	t.Run("not full mode returns empty", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)

		result, err := EnterCloudProviderOptions(prompter, false, true, nil, nil)
		require.NoError(t, err)
		require.Nil(t, result.Azure)
		require.Nil(t, result.AWS)
	})

	t.Run("not creating environment returns empty", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)

		result, err := EnterCloudProviderOptions(prompter, true, false, nil, nil)
		require.NoError(t, err)
		require.Nil(t, result.Azure)
		require.Nil(t, result.AWS)
	})

	t.Run("user declines cloud provider", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)

		prompter.EXPECT().
			GetListInput(gomock.Any(), gomock.Any()).
			Return(prompt.ConfirmNo, nil).Times(1)

		result, err := EnterCloudProviderOptions(prompter, true, true, nil, nil)
		require.NoError(t, err)
		require.Nil(t, result.Azure)
		require.Nil(t, result.AWS)
	})

	t.Run("user navigates back", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)

		// First: confirm yes to add cloud provider
		prompter.EXPECT().
			GetListInput(gomock.Any(), gomock.Any()).
			Return(prompt.ConfirmYes, nil).Times(1)

		// Second: select [back]
		prompter.EXPECT().
			GetListInput(gomock.Any(), SelectCloudProviderPrompt).
			Return(ConfirmCloudProviderBackNavigationSentinel, nil).Times(1)

		result, err := EnterCloudProviderOptions(prompter, true, true,
			func() (*azure.Provider, error) { return nil, nil },
			func() (*aws.Provider, error) { return nil, nil },
		)
		require.NoError(t, err)
		require.Nil(t, result.Azure)
		require.Nil(t, result.AWS)
	})
}
