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

func Test_Run_Fail(t *testing.T) {

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
		var args []string

		runner.EXPECT().Validate(gomock.Any(), gomock.Any()).Times(1)
		runner.EXPECT().Run(gomock.Any()).Return(errors.New("mock error"))

		fn := RunCommand(runner)
		fn(testCmd, args)

		require.Error(t, expected)

	})
}
