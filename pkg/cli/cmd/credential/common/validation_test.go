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
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/cmd/validation"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

const (
	defaultTestValue = "default"
	testEnvName      = "env0"
	invalidEnvName   = "&^*env"
)

func Test_Environment_Selection(t *testing.T) {
	testCmd := &cobra.Command{}
	testCmd.Flags().String("environment", testEnvName, "Environment flag")
	testEmptyEnvCmd := &cobra.Command{}
	testEmptyEnvCmd.Flags().String("environment", "", "Environment flag")
	testInvalidEnvNameCmd := &cobra.Command{}
	testInvalidEnvNameCmd.Flags().String("environment", invalidEnvName, "Environment flag")

	tests := []struct {
		name        string
		cmd         *cobra.Command
		defaultVal  string
		interactive bool
		expectedEnv string
		err         error
		mockSetup   func(*prompt.MockInterface)
	}{
		{
			name:        "Select environment non interactive",
			cmd:         testCmd,
			defaultVal:  defaultTestValue,
			interactive: false,
			expectedEnv: testEnvName,
		},
		{
			name:        "Default environment non interactive",
			cmd:         testEmptyEnvCmd,
			defaultVal:  defaultTestValue,
			interactive: false,
			expectedEnv: defaultTestValue,
		},
		{
			name:        "Undefined environment flag non interactive",
			cmd:         &cobra.Command{},
			defaultVal:  defaultTestValue,
			interactive: false,
			err:         errors.New("flag accessed but not defined: environment"),
		},
		{
			name:        "Invalid environment name non interactive",
			cmd:         testInvalidEnvNameCmd,
			defaultVal:  invalidEnvName,
			interactive: false,
			err:         fmt.Errorf("%s %s. Use --environment option to specify the valid name", invalidEnvName, prompt.InvalidResourceNameMessage),
		},
		{
			name:        "environment name interactive",
			cmd:         testEmptyEnvCmd,
			defaultVal:  defaultTestValue,
			interactive: true,
			expectedEnv: testEnvName,
			err:         nil,
			mockSetup:   func(m *prompt.MockInterface) { setupEnvNameTextPrompt(m, testEnvName) },
		},
		{
			name:        "environment name interactive",
			cmd:         testEmptyEnvCmd,
			defaultVal:  defaultTestValue,
			interactive: true,
			expectedEnv: defaultTestValue,
			err:         nil,
			mockSetup:   func(m *prompt.MockInterface) { setupEnvNameTextPrompt(m, "") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			prompter := prompt.NewMockInterface(ctrl)
			if tt.mockSetup != nil {
				tt.mockSetup(prompter)
			}
			selected, err := validation.SelectEnvironmentName(tt.cmd, tt.defaultVal, tt.interactive, prompter)
			if tt.err != nil {
				require.Equal(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, selected, tt.expectedEnv)
			}
		})
	}
}

func setupEnvNameTextPrompt(prompter *prompt.MockInterface, value string) {
	prompter.EXPECT().
		GetTextInput(validation.EnterEnvironmentNamePrompt, gomock.Any()).
		Return(value, nil).Times(1)
}
