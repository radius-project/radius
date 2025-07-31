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

package kubernetes

import (
	"context"
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestNewCommand(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFactory := framework.NewMockFactory(ctrl)
	mockHelm := helm.NewMockInterface(ctrl)
	mockOutput := output.NewMockInterface(ctrl)

	mockFactory.EXPECT().GetHelmInterface().Return(mockHelm)
	mockFactory.EXPECT().GetOutput().Return(mockOutput)

	cmd, runner := NewCommand(mockFactory)

	require.NotNil(t, cmd)
	require.NotNil(t, runner)
	assert.Equal(t, "kubernetes", cmd.Use)
	assert.Equal(t, "Rolls back Radius on a Kubernetes cluster", cmd.Short)
}

func TestRunner_Validate(t *testing.T) {
	t.Parallel()
	runner := &Runner{}
	err := runner.Validate(nil, nil)
	require.NoError(t, err)
}

func TestRunner_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		runner        Runner
		mockSetup     func(helmMock *helm.MockInterface, outputMock *output.MockInterface)
		expectedError string
	}{
		{
			name: "revision 0 - triggers automatic rollback",
			runner: Runner{
				KubeContext: "test-context",
				Revision:    0,
			},
			mockSetup: func(helmMock *helm.MockInterface, outputMock *output.MockInterface) {
				// Mock CheckRadiusInstall
				helmMock.EXPECT().
					CheckRadiusInstall("test-context").
					Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "v0.46.0"}, nil).
					Times(1)

				// Mock output calls - should use automatic rollback path
				outputMock.EXPECT().LogInfo("Current Radius version: %s", "v0.46.0").Times(1)
				outputMock.EXPECT().LogInfo("Checking for previous revisions...").Times(1)
				outputMock.EXPECT().LogInfo("✓ Radius rollback completed successfully!").Times(1)

				// Mock RollbackRadius (not RollbackRadiusToRevision)
				helmMock.EXPECT().
					RollbackRadius(gomock.Any(), "test-context").
					Return(nil).
					Times(1)
			},
			expectedError: "",
		},
		{
			name: "no revision specified - automatic rollback success",
			runner: Runner{
				KubeContext: "test-context",
			},
			mockSetup: func(helmMock *helm.MockInterface, outputMock *output.MockInterface) {
				helmMock.EXPECT().
					CheckRadiusInstall("test-context").
					Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "v0.46.0"}, nil).
					Times(1)

				outputMock.EXPECT().LogInfo("Current Radius version: %s", "v0.46.0").Times(1)
				outputMock.EXPECT().LogInfo("Checking for previous revisions...").Times(1)
				outputMock.EXPECT().LogInfo("✓ Radius rollback completed successfully!").Times(1)

				helmMock.EXPECT().
					RollbackRadius(gomock.Any(), "test-context").
					Return(nil).
					Times(1)
			},
			expectedError: "",
		},
		{
			name: "specific revision 3 - success",
			runner: Runner{
				KubeContext: "test-context",
				Revision:    3,
			},
			mockSetup: func(helmMock *helm.MockInterface, outputMock *output.MockInterface) {
				helmMock.EXPECT().
					CheckRadiusInstall("test-context").
					Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "v0.46.0"}, nil).
					Times(1)

				outputMock.EXPECT().LogInfo("Current Radius version: %s", "v0.46.0").Times(1)
				outputMock.EXPECT().LogInfo("Rolling back to specified revision %d...", 3).Times(1)
				outputMock.EXPECT().LogInfo("✓ Radius rollback completed successfully!").Times(1)

				helmMock.EXPECT().
					RollbackRadiusToRevision(gomock.Any(), "test-context", 3).
					Return(nil).
					Times(1)
			},
			expectedError: "",
		},
		{
			name: "specific revision 999 - not found",
			runner: Runner{
				KubeContext: "test-context",
				Revision:    999,
			},
			mockSetup: func(helmMock *helm.MockInterface, outputMock *output.MockInterface) {
				helmMock.EXPECT().
					CheckRadiusInstall("test-context").
					Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "v0.46.0"}, nil).
					Times(1)

				outputMock.EXPECT().LogInfo("Current Radius version: %s", "v0.46.0").Times(1)
				outputMock.EXPECT().LogInfo("Rolling back to specified revision %d...", 999).Times(1)

				helmMock.EXPECT().
					RollbackRadiusToRevision(gomock.Any(), "test-context", 999).
					Return(errors.New("revision not found")).
					Times(1)
			},
			expectedError: "failed to rollback Radius to revision 999",
		},
		{
			name: "radius not installed",
			runner: Runner{
				KubeContext: "test-context",
			},
			mockSetup: func(helmMock *helm.MockInterface, outputMock *output.MockInterface) {
				helmMock.EXPECT().
					CheckRadiusInstall("test-context").
					Return(helm.InstallState{RadiusInstalled: false}, nil).
					Times(1)
			},
			expectedError: "Radius is not currently installed. Use 'rad install kubernetes' to install Radius first",
		},
		{
			name: "check install fails",
			runner: Runner{
				KubeContext: "test-context",
			},
			mockSetup: func(helmMock *helm.MockInterface, outputMock *output.MockInterface) {
				helmMock.EXPECT().
					CheckRadiusInstall("test-context").
					Return(helm.InstallState{}, errors.New("helm error")).
					Times(1)
			},
			expectedError: "failed to check current Radius installation",
		},
		{
			name: "automatic rollback fails",
			runner: Runner{
				KubeContext: "test-context",
			},
			mockSetup: func(helmMock *helm.MockInterface, outputMock *output.MockInterface) {
				helmMock.EXPECT().
					CheckRadiusInstall("test-context").
					Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "v0.46.0"}, nil).
					Times(1)

				outputMock.EXPECT().LogInfo("Current Radius version: %s", "v0.46.0").Times(1)
				outputMock.EXPECT().LogInfo("Checking for previous revisions...").Times(1)

				helmMock.EXPECT().
					RollbackRadius(gomock.Any(), "test-context").
					Return(errors.New("rollback failed")).
					Times(1)
			},
			expectedError: "failed to rollback Radius",
		},
		{
			name: "specific revision rollback fails",
			runner: Runner{
				KubeContext: "test-context",
				Revision:    5,
			},
			mockSetup: func(helmMock *helm.MockInterface, outputMock *output.MockInterface) {
				helmMock.EXPECT().
					CheckRadiusInstall("test-context").
					Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "v0.46.0"}, nil).
					Times(1)

				outputMock.EXPECT().LogInfo("Current Radius version: %s", "v0.46.0").Times(1)
				outputMock.EXPECT().LogInfo("Rolling back to specified revision %d...", 5).Times(1)

				helmMock.EXPECT().
					RollbackRadiusToRevision(gomock.Any(), "test-context", 5).
					Return(errors.New("rollback operation failed")).
					Times(1)
			},
			expectedError: "failed to rollback Radius to revision 5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			helmMock := helm.NewMockInterface(ctrl)
			outputMock := output.NewMockInterface(ctrl)

			runner := tt.runner
			runner.Helm = helmMock
			runner.Output = outputMock

			tt.mockSetup(helmMock, outputMock)

			err := runner.Run(context.Background())

			if tt.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestNewRunner(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFactory := framework.NewMockFactory(ctrl)
	helmMock := helm.NewMockInterface(ctrl)
	outputMock := output.NewMockInterface(ctrl)

	mockFactory.EXPECT().GetHelmInterface().Return(helmMock).Times(1)
	mockFactory.EXPECT().GetOutput().Return(outputMock).Times(1)

	runner := NewRunner(mockFactory)

	require.Same(t, helmMock, runner.Helm)
	require.Same(t, outputMock, runner.Output)
}

func TestRunner_Run_ListRevisions(t *testing.T) {
	t.Parallel()

	revisions := []helm.RevisionInfo{
		{
			Revision:     2,
			ChartVersion: "0.46.0",
			Status:       "deployed",
			UpdatedAt:    "2023-01-01 12:00:00",
			Description:  "Upgrade complete",
		},
		{
			Revision:     1,
			ChartVersion: "0.45.0",
			Status:       "superseded",
			UpdatedAt:    "2023-01-01 11:00:00",
			Description:  "Install complete",
		},
	}

	tests := []struct {
		name          string
		runner        Runner
		mockSetup     func(helmMock *helm.MockInterface, outputMock *output.MockInterface)
		expectedError string
	}{
		{
			name: "list revisions success",
			runner: Runner{
				KubeContext:   "test-context",
				ListRevisions: true,
			},
			mockSetup: func(helmMock *helm.MockInterface, outputMock *output.MockInterface) {
				helmMock.EXPECT().
					CheckRadiusInstall("test-context").
					Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "v0.46.0"}, nil).
					Times(1)

				outputMock.EXPECT().LogInfo("Current Radius version: %s", "v0.46.0").Times(1)

				helmMock.EXPECT().
					GetRadiusRevisions(gomock.Any(), "test-context").
					Return(revisions, nil).
					Times(1)

				outputMock.EXPECT().
					WriteFormatted("table", revisions, gomock.Any()).
					Return(nil).
					Times(1)
			},
			expectedError: "",
		},
		{
			name: "list revisions error",
			runner: Runner{
				KubeContext:   "test-context",
				ListRevisions: true,
			},
			mockSetup: func(helmMock *helm.MockInterface, outputMock *output.MockInterface) {
				helmMock.EXPECT().
					CheckRadiusInstall("test-context").
					Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "v0.46.0"}, nil).
					Times(1)

				outputMock.EXPECT().LogInfo("Current Radius version: %s", "v0.46.0").Times(1)

				helmMock.EXPECT().
					GetRadiusRevisions(gomock.Any(), "test-context").
					Return(nil, errors.New("failed to get history")).
					Times(1)
			},
			expectedError: "failed to get revision history",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			helmMock := helm.NewMockInterface(ctrl)
			outputMock := output.NewMockInterface(ctrl)

			runner := tt.runner
			runner.Helm = helmMock
			runner.Output = outputMock

			tt.mockSetup(helmMock, outputMock)

			err := runner.Run(context.Background())

			if tt.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}
