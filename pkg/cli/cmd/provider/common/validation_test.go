// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package common

import (
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

const (
	DefaultTestValue = "default"
	TestEnvName      = "env0"
	InvalidEnvName   = "&^*env"
)

func Test_Environment_Selection(t *testing.T) {
	testCmd := &cobra.Command{}
	testCmd.Flags().String("environment", TestEnvName, "Environment flag")
	testEmptyEnvCmd := &cobra.Command{}
	testEmptyEnvCmd.Flags().String("environment", "", "Environment flag")
	testInvalidEnvNameCmd := &cobra.Command{}
	testInvalidEnvNameCmd.Flags().String("environment", InvalidEnvName, "Environment flag")

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
			defaultVal:  DefaultTestValue,
			interactive: false,
			expectedEnv: TestEnvName,
		},
		{
			name:        "Default environment non interactive",
			cmd:         testEmptyEnvCmd,
			defaultVal:  DefaultTestValue,
			interactive: false,
			expectedEnv: DefaultTestValue,
		},
		{
			name:        "Undefined environment flag non interactive",
			cmd:         &cobra.Command{},
			defaultVal:  DefaultTestValue,
			interactive: false,
			err:         errors.New("flag accessed but not defined: environment"),
		},
		{
			name:        "Invalid environment name non interactive",
			cmd:         testInvalidEnvNameCmd,
			defaultVal:  InvalidEnvName,
			interactive: false,
			err:         fmt.Errorf("%s %s. Use --environment option to specify the valid name", InvalidEnvName, prompt.InvalidResourceNameMessage),
		},
		{
			name:        "environment name interactive",
			cmd:         testEmptyEnvCmd,
			defaultVal:  DefaultTestValue,
			interactive: true,
			expectedEnv: TestEnvName,
			err:         nil,
			mockSetup:   setupEnvNameTextPrompt,
		},
		{
			name:        "environment name interactive",
			cmd:         testEmptyEnvCmd,
			defaultVal:  DefaultTestValue,
			interactive: true,
			expectedEnv: DefaultTestValue,
			err:         nil,
			mockSetup:   setupEmptyEnvNameTextPrompt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			prompter := prompt.NewMockInterface(ctrl)
			if tt.mockSetup != nil {
				tt.mockSetup(prompter)
			}
			selected, err := SelectEnvironmentName(tt.cmd, tt.defaultVal, tt.interactive, prompter)
			if tt.err != nil {
				require.Equal(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, selected, tt.expectedEnv)
			}
		})
	}
}

func setupEnvNameTextPrompt(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetTextInput(EnterEnvironmentNamePrompt, gomock.Any()).
		Return(TestEnvName, nil).Times(1)
}

func setupEmptyEnvNameTextPrompt(prompter *prompt.MockInterface) {
	prompter.EXPECT().
		GetTextInput(EnterEnvironmentNamePrompt, gomock.Any()).
		Return("", nil).Times(1)
}
