/*
Copyright 2025 The Radius Authors.

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

package preupgrade

import (
	"context"
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRunPreflightChecks_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelm := helm.NewMockInterface(ctrl)
	mockOutput := output.NewMockInterface(ctrl)

	installState := helm.InstallState{
		RadiusInstalled: true,
		RadiusVersion:   "0.28.0",
	}

	mockHelm.EXPECT().CheckRadiusInstall("test-context").Return(installState, nil)
	mockOutput.EXPECT().LogInfo("Running preflight checks: %s", "version")
	mockOutput.EXPECT().LogInfo("Running pre-flight checks...")
	mockOutput.EXPECT().LogInfo("  Running %s...", gomock.Any())
	mockOutput.EXPECT().LogInfo("    %s %s", gomock.Any(), gomock.Any())
	mockOutput.EXPECT().LogInfo("Pre-flight checks completed successfully")
	mockOutput.EXPECT().LogInfo("All preflight checks completed successfully")
	mockOutput.EXPECT().LogInfo("✓ %s: %s", gomock.Any(), gomock.Any())

	config := Config{
		KubeContext: "test-context",
		Helm:        mockHelm,
		Output:      mockOutput,
	}

	options := Options{
		EnabledChecks: []string{"version"},
		TargetVersion: "0.29.0",
	}

	err := RunPreflightChecks(context.Background(), config, options)
	require.NoError(t, err)
}

func TestRunPreflightChecks_MultipleChecks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelm := helm.NewMockInterface(ctrl)
	mockOutput := output.NewMockInterface(ctrl)

	installState := helm.InstallState{
		RadiusInstalled: true,
		RadiusVersion:   "0.28.0",
	}

	mockHelm.EXPECT().CheckRadiusInstall("test-context").Return(installState, nil).Times(2)
	mockOutput.EXPECT().LogInfo("Running preflight checks: %s", "version, version")
	mockOutput.EXPECT().LogInfo("Running pre-flight checks...")
	mockOutput.EXPECT().LogInfo("  Running %s...", gomock.Any()).Times(2)
	mockOutput.EXPECT().LogInfo("    %s %s", gomock.Any(), gomock.Any()).Times(2)
	mockOutput.EXPECT().LogInfo("Pre-flight checks completed successfully")
	mockOutput.EXPECT().LogInfo("All preflight checks completed successfully")
	mockOutput.EXPECT().LogInfo("✓ %s: %s", gomock.Any(), gomock.Any()).Times(2)

	config := Config{
		KubeContext: "test-context",
		Helm:        mockHelm,
		Output:      mockOutput,
	}

	options := Options{
		EnabledChecks: []string{"version", "version"},
		TargetVersion: "0.29.0",
	}

	err := RunPreflightChecks(context.Background(), config, options)
	require.NoError(t, err)
}

func TestRunPreflightChecks_WithSpacesInCheckNames(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelm := helm.NewMockInterface(ctrl)
	mockOutput := output.NewMockInterface(ctrl)

	installState := helm.InstallState{
		RadiusInstalled: true,
		RadiusVersion:   "0.28.0",
	}

	mockHelm.EXPECT().CheckRadiusInstall("test-context").Return(installState, nil)
	mockOutput.EXPECT().LogInfo("Running preflight checks: %s", " version ")
	mockOutput.EXPECT().LogInfo("Running pre-flight checks...")
	mockOutput.EXPECT().LogInfo("  Running %s...", gomock.Any())
	mockOutput.EXPECT().LogInfo("    %s %s", gomock.Any(), gomock.Any())
	mockOutput.EXPECT().LogInfo("Pre-flight checks completed successfully")
	mockOutput.EXPECT().LogInfo("All preflight checks completed successfully")
	mockOutput.EXPECT().LogInfo("✓ %s: %s", gomock.Any(), gomock.Any())

	config := Config{
		KubeContext: "test-context",
		Helm:        mockHelm,
		Output:      mockOutput,
	}

	options := Options{
		EnabledChecks: []string{" version "},
		TargetVersion: "0.29.0",
	}

	err := RunPreflightChecks(context.Background(), config, options)
	require.NoError(t, err)
}

func TestRunPreflightChecks_HelmCheckError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelm := helm.NewMockInterface(ctrl)
	mockOutput := output.NewMockInterface(ctrl)

	expectedError := errors.New("failed to connect to kubernetes")
	mockHelm.EXPECT().CheckRadiusInstall("test-context").Return(helm.InstallState{}, expectedError)
	mockOutput.EXPECT().LogInfo("Running preflight checks: %s", "version")

	config := Config{
		KubeContext: "test-context",
		Helm:        mockHelm,
		Output:      mockOutput,
	}

	options := Options{
		EnabledChecks: []string{"version"},
		TargetVersion: "0.29.0",
	}

	err := RunPreflightChecks(context.Background(), config, options)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check current Radius installation")
	assert.Contains(t, err.Error(), "failed to connect to kubernetes")
}

func TestRunPreflightChecks_UnknownCheckName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelm := helm.NewMockInterface(ctrl)
	mockOutput := output.NewMockInterface(ctrl)

	mockOutput.EXPECT().LogInfo("Running preflight checks: %s", "unknown-check")

	config := Config{
		KubeContext: "test-context",
		Helm:        mockHelm,
		Output:      mockOutput,
	}

	options := Options{
		EnabledChecks: []string{"unknown-check"},
		TargetVersion: "0.29.0",
	}

	err := RunPreflightChecks(context.Background(), config, options)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown check 'unknown-check'")
}

func TestRunPreflightChecks_EmptyChecksList(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelm := helm.NewMockInterface(ctrl)
	mockOutput := output.NewMockInterface(ctrl)

	mockOutput.EXPECT().LogInfo("Running preflight checks: %s", "")
	mockOutput.EXPECT().LogInfo("All preflight checks completed successfully")

	config := Config{
		KubeContext: "test-context",
		Helm:        mockHelm,
		Output:      mockOutput,
	}

	options := Options{
		EnabledChecks: []string{},
		TargetVersion: "0.29.0",
	}

	err := RunPreflightChecks(context.Background(), config, options)
	require.NoError(t, err)
}

func TestRunPreflightChecks_PreflightCheckFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelm := helm.NewMockInterface(ctrl)
	mockOutput := output.NewMockInterface(ctrl)

	installState := helm.InstallState{
		RadiusInstalled: true,
		RadiusVersion:   "0.30.0",
	}

	mockHelm.EXPECT().CheckRadiusInstall("test-context").Return(installState, nil)
	mockOutput.EXPECT().LogInfo("Running preflight checks: %s", "version")
	mockOutput.EXPECT().LogInfo("Running pre-flight checks...")
	mockOutput.EXPECT().LogInfo("  Running %s...", gomock.Any())
	mockOutput.EXPECT().LogInfo("    %s [ERROR] %s", gomock.Any(), gomock.Any())

	config := Config{
		KubeContext: "test-context",
		Helm:        mockHelm,
		Output:      mockOutput,
	}

	options := Options{
		EnabledChecks: []string{"version"},
		TargetVersion: "0.29.0",
	}

	err := RunPreflightChecks(context.Background(), config, options)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pre-flight check")
	assert.Contains(t, err.Error(), "failed")
}