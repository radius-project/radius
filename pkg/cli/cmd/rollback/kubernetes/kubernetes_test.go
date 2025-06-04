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
	runner := &Runner{}
	err := runner.Validate(nil, nil)
	require.NoError(t, err)
}

func TestRunner_Run_SpecificRevision_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	helmMock := helm.NewMockInterface(ctrl)
	outputMock := output.NewMockInterface(ctrl)

	runner := &Runner{
		Helm:        helmMock,
		Output:      outputMock,
		KubeContext: "test-context",
		Revision:    3,
	}

	// Mock CheckRadiusInstall to return that Radius is installed
	helmMock.EXPECT().
		CheckRadiusInstall("test-context").
		Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "v0.46.0"}, nil).
		Times(1)

	// Mock output calls
	outputMock.EXPECT().LogInfo("Current Radius version: %s", "v0.46.0").Times(1)
	outputMock.EXPECT().LogInfo("Rolling back to specified revision %d...", 3).Times(1)
	outputMock.EXPECT().LogInfo("✓ Radius rollback completed successfully!").Times(1)

	// Mock RollbackRadiusToRevision to succeed
	helmMock.EXPECT().
		RollbackRadiusToRevision(gomock.Any(), "test-context", 3).
		Return(nil).
		Times(1)

	err := runner.Run(context.Background())
	require.NoError(t, err)
}

func TestRunner_Run_SpecificRevision_Fails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	helmMock := helm.NewMockInterface(ctrl)
	outputMock := output.NewMockInterface(ctrl)

	runner := &Runner{
		Helm:        helmMock,
		Output:      outputMock,
		KubeContext: "test-context",
		Revision:    999,
	}

	// Mock CheckRadiusInstall to return that Radius is installed
	helmMock.EXPECT().
		CheckRadiusInstall("test-context").
		Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "v0.46.0"}, nil).
		Times(1)

	// Mock output calls
	outputMock.EXPECT().LogInfo("Current Radius version: %s", "v0.46.0").Times(1)
	outputMock.EXPECT().LogInfo("Rolling back to specified revision %d...", 999).Times(1)

	// Mock RollbackRadiusToRevision to fail
	helmMock.EXPECT().
		RollbackRadiusToRevision(gomock.Any(), "test-context", 999).
		Return(errors.New("revision not found")).
		Times(1)

	err := runner.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to rollback Radius to revision 999")
}

func TestRunner_Run_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	helmMock := helm.NewMockInterface(ctrl)
	outputMock := output.NewMockInterface(ctrl)

	runner := &Runner{
		Helm:        helmMock,
		Output:      outputMock,
		KubeContext: "test-context",
	}

	// Mock CheckRadiusInstall to return that Radius is installed
	helmMock.EXPECT().
		CheckRadiusInstall("test-context").
		Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "v0.46.0"}, nil).
		Times(1)

	// Mock output calls
	outputMock.EXPECT().LogInfo("Current Radius version: %s", "v0.46.0").Times(1)
	outputMock.EXPECT().LogInfo("Checking for previous revisions...").Times(1)
	outputMock.EXPECT().LogInfo("✓ Radius rollback completed successfully!").Times(1)

	// Mock RollbackRadius to succeed
	helmMock.EXPECT().
		RollbackRadius(gomock.Any(), "test-context").
		Return(nil).
		Times(1)

	err := runner.Run(context.Background())
	require.NoError(t, err)
}

func TestRunner_Run_RadiusNotInstalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	helmMock := helm.NewMockInterface(ctrl)
	outputMock := output.NewMockInterface(ctrl)

	runner := &Runner{
		Helm:        helmMock,
		Output:      outputMock,
		KubeContext: "test-context",
	}

	// Mock CheckRadiusInstall to return that Radius is not installed
	helmMock.EXPECT().
		CheckRadiusInstall("test-context").
		Return(helm.InstallState{RadiusInstalled: false}, nil).
		Times(1)

	err := runner.Run(context.Background())
	require.Error(t, err)
	require.Equal(t, "Radius is not currently installed. Use 'rad install kubernetes' to install Radius first", err.Error())
}

func TestRunner_Run_CheckInstallFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	helmMock := helm.NewMockInterface(ctrl)
	outputMock := output.NewMockInterface(ctrl)

	runner := &Runner{
		Helm:        helmMock,
		Output:      outputMock,
		KubeContext: "test-context",
	}

	// Mock CheckRadiusInstall to fail
	helmMock.EXPECT().
		CheckRadiusInstall("test-context").
		Return(helm.InstallState{}, errors.New("helm error")).
		Times(1)

	err := runner.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to check current Radius installation")
}

func TestRunner_Run_RollbackFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	helmMock := helm.NewMockInterface(ctrl)
	outputMock := output.NewMockInterface(ctrl)

	runner := &Runner{
		Helm:        helmMock,
		Output:      outputMock,
		KubeContext: "test-context",
	}

	// Mock CheckRadiusInstall to return that Radius is installed
	helmMock.EXPECT().
		CheckRadiusInstall("test-context").
		Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "v0.46.0"}, nil).
		Times(1)

	// Mock output calls
	outputMock.EXPECT().LogInfo("Current Radius version: %s", "v0.46.0").Times(1)
	outputMock.EXPECT().LogInfo("Checking for previous revisions...").Times(1)

	// Mock RollbackRadius to fail
	helmMock.EXPECT().
		RollbackRadius(gomock.Any(), "test-context").
		Return(errors.New("rollback failed")).
		Times(1)

	err := runner.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to rollback Radius")
}

func TestNewRunner(t *testing.T) {
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
