// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package framework

import (
	"errors"
	"testing"

	gomock "github.com/golang/mock/gomock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func Test_RunCommand_Fail(t *testing.T) {
	t.Run("Run runner", func(t *testing.T) {
		expected := errors.New("mock error")
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

		require.ErrorIs(t, errors.Unwrap(err), expected)

	})
}
