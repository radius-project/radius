// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package framework

import (
	"testing"

	gomock "github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func Test_RunCommand_Fail(t *testing.T) {
	t.Run("Run runner validation succeeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		runner := NewMockRunner(ctrl)
		testCmd := &cobra.Command{
			Use:   "test",
			Short: "short description",
			Long:  `long description`,
			Run: func(cmd *cobra.Command, args []string) {
			},
		}
		var testArgs []string

		runner.EXPECT().Validate(gomock.Any(), gomock.Any()).Times(1)
		runner.EXPECT().Run(gomock.Any()).Times(1)

		fn := RunCommand(runner)
		err := fn(testCmd, testArgs)

		require.NoError(t, err)

	})

	t.Run("Run runner validation fails", func(t *testing.T) {
		expected := &cli.FriendlyError{}
		ctrl := gomock.NewController(t)
		runner := NewMockRunner(ctrl)
		testCmd := &cobra.Command{
			Use:   "test",
			Short: "short description",
			Long:  `long description`,
			Run: func(cmd *cobra.Command, args []string) {
			},
		}
		var testArgs []string

		runner.EXPECT().Validate(gomock.Any(), gomock.Any()).Times(1)
		runner.EXPECT().Run(gomock.Any()).Return(expected)

		fn := RunCommand(runner)
		err := fn(testCmd, testArgs)

		require.ErrorIs(t, err, expected)

	})
}
