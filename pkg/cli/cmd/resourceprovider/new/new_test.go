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

package new

import (
	"context"
	"os"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	config := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid",
			Input:         []string{"Applications.Test"},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: missing args",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: too many args",
			Input:         []string{"Applications.Test", "dddd"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	expected, err := os.ReadFile("testdata/expected-output.json")
	require.NoError(t, err)

	t.Run("Success: Scaffold resource provider", func(t *testing.T) {
		original, err := os.Getwd()
		require.NoError(t, err)

		directory := t.TempDir()
		require.NoError(t, os.Chdir(directory))
		defer func() {
			require.NoError(t, os.Chdir(original))
		}()

		mockOutput := &output.MockOutput{}

		runner := &Runner{
			Output:                    mockOutput,
			ResourceProviderNamespace: "Applications.Test",
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)

		actual, err := os.ReadFile("Applications.Test.json")
		require.NoError(t, err)
		require.Equal(t, string(expected), string(actual))

		expectedOutput := []any{
			output.LogOutput{
				Format: "Wrote template to: %s",
				Params: []any{"Applications.Test.json"},
			},
		}
		require.Equal(t, expectedOutput, mockOutput.Writes)
	})

	t.Run("Success: File exists -> force", func(t *testing.T) {
		original, err := os.Getwd()
		require.NoError(t, err)

		directory := t.TempDir()
		require.NoError(t, os.Chdir(directory))
		defer func() {
			require.NoError(t, os.Chdir(original))
		}()

		mockOutput := &output.MockOutput{}

		runner := &Runner{
			Output:                    mockOutput,
			ResourceProviderNamespace: "Applications.Test",
			Force:                     true,
		}

		err = os.WriteFile("Applications.Test.json", []byte("{}"), 0644)
		require.NoError(t, err)

		err = runner.Run(context.Background())
		require.NoError(t, err)

		actual, err := os.ReadFile("Applications.Test.json")
		require.NoError(t, err)
		require.Equal(t, string(expected), string(actual))

		expectedOutput := []any{
			output.LogOutput{
				Format: "Wrote template to: %s",
				Params: []any{"Applications.Test.json"},
			},
		}
		require.Equal(t, expectedOutput, mockOutput.Writes)
	})

	t.Run("Error: File exists -> canceled", func(t *testing.T) {
		original, err := os.Getwd()
		require.NoError(t, err)

		directory := t.TempDir()
		require.NoError(t, os.Chdir(directory))
		defer func() {
			require.NoError(t, os.Chdir(original))
		}()

		mockOutput := &output.MockOutput{}

		runner := &Runner{
			Output:                    mockOutput,
			ResourceProviderNamespace: "Applications.Test",
		}

		err = os.WriteFile("Applications.Test.json", []byte("{}"), 0644)
		require.NoError(t, err)

		err = runner.Run(context.Background())
		require.Equal(t, err, clierrors.Message("File \"Applications.Test.json\" already exists, use --force to overwrite."))
	})
}
