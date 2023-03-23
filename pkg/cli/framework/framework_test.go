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

func Test_RunCommand(t *testing.T) {
	validationErr := errors.New("validation error")
	runErr := errors.New("run error")

	testCases := []struct {
		testname      string
		validationErr error
		runErr        error
		expectedErr   error
	}{
		{
			testname:      "test-run-command-pass",
			validationErr: nil,
			runErr:        nil,
			expectedErr:   nil,
		},
		{
			testname:      "test-run-command-validation-fail",
			validationErr: validationErr,
			expectedErr:   validationErr,
		},
		{
			testname:      "test-run-command-run-fail",
			validationErr: nil,
			runErr:        runErr,
			expectedErr:   runErr,
		},
	}

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

	for _, tt := range testCases {
		t.Run(tt.testname, func(t *testing.T) {
			runner.EXPECT().Validate(gomock.Any(), gomock.Any()).Times(1).Return(tt.validationErr)
			if tt.validationErr == nil {
				runner.EXPECT().Run(gomock.Any()).Times(1).Return(tt.runErr)
			}

			fn := RunCommand(runner)
			err := fn(testCmd, testArgs)

			require.Equal(t, tt.expectedErr, err)
		})
	}
}
