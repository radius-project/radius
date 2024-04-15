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

package radinit

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/stretchr/testify/require"
)

func Test_enterApplicationOptions(t *testing.T) {
	t.Run("create application: Yes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter}

		setScaffoldApplicationPromptYes(prompter)

		options := initOptions{}
		err := runner.enterApplicationOptions(context.Background(), &options)
		require.NoError(t, err)

		require.Equal(t, applicationOptions{Scaffold: true, Name: "radinit"}, options.Application)
	})
	t.Run("create application: No", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter}

		setScaffoldApplicationPromptNo(prompter)

		options := initOptions{}
		err := runner.enterApplicationOptions(context.Background(), &options)
		require.NoError(t, err)

		require.Equal(t, applicationOptions{Scaffold: false, Name: ""}, options.Application)
	})
}

func Test_enterApplicationName(t *testing.T) {
	t.Run("default is valid", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter}

		name, err := runner.enterApplicationName(func() (string, error) { return "valid", nil })
		require.NoError(t, err)
		require.Equal(t, "valid", name)
	})
	t.Run("user is prompted", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter}

		setApplicationNamePrompt(prompter, "another-name")

		name, err := runner.enterApplicationName(func() (string, error) { return "invalid-0-----", nil })
		require.NoError(t, err)
		require.Equal(t, "another-name", name)
	})

}
