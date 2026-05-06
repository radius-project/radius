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

	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_EnterApplicationOptions(t *testing.T) {
	t.Run("scaffold yes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)

		prompter.EXPECT().
			GetListInput(gomock.Any(), ConfirmSetupApplicationPrompt).
			Return(prompt.ConfirmYes, nil).Times(1)

		scaffold, name, err := EnterApplicationOptions(prompter)
		require.NoError(t, err)
		require.True(t, scaffold)
		require.NotEmpty(t, name)
	})

	t.Run("scaffold no", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)

		prompter.EXPECT().
			GetListInput(gomock.Any(), ConfirmSetupApplicationPrompt).
			Return(prompt.ConfirmNo, nil).Times(1)

		scaffold, name, err := EnterApplicationOptions(prompter)
		require.NoError(t, err)
		require.False(t, scaffold)
		require.Empty(t, name)
	})
}

func Test_EnterApplicationName(t *testing.T) {
	t.Run("default is valid", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)

		name, err := EnterApplicationName(prompter, func() (string, error) { return "valid", nil })
		require.NoError(t, err)
		require.Equal(t, "valid", name)
	})

	t.Run("user is prompted when default is invalid", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)

		prompter.EXPECT().
			GetTextInput(EnterApplicationNamePrompt, gomock.Any()).
			Return("another-name", nil).Times(1)

		name, err := EnterApplicationName(prompter, func() (string, error) { return "invalid-0-----", nil })
		require.NoError(t, err)
		require.Equal(t, "another-name", name)
	})

	t.Run("user is prompted when application name contains uppercase", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)

		prompter.EXPECT().
			GetTextInput(EnterApplicationNamePrompt, gomock.Any()).
			Return("another-name", nil).Times(1)

		name, err := EnterApplicationName(prompter, func() (string, error) { return "Invalid-Name", nil })
		require.NoError(t, err)
		require.Equal(t, "another-name", name)
	})

	t.Run("user is prompted when application name does not end with alphanumeric", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)

		prompter.EXPECT().
			GetTextInput(EnterApplicationNamePrompt, gomock.Any()).
			Return("another-name", nil).Times(1)

		name, err := EnterApplicationName(prompter, func() (string, error) { return "test-application-", nil })
		require.NoError(t, err)
		require.Equal(t, "another-name", name)
	})

	t.Run("user is prompted when application name is too long", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)

		prompter.EXPECT().
			GetTextInput(EnterApplicationNamePrompt, gomock.Any()).
			Return("another-name", nil).Times(1)

		name, err := EnterApplicationName(prompter, func() (string, error) {
			return "this-is-a-very-long-environment-name-that-is-invalid-this-is-a-very-long-application-name-that-is-invalid", nil
		})
		require.NoError(t, err)
		require.Equal(t, "another-name", name)
	})
}
