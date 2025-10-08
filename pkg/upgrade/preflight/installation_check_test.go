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

package preflight

import (
	"context"
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRadiusInstallationCheck_Run(t *testing.T) {
	tests := []struct {
		name          string
		installState  helm.InstallState
		helmError     error
		expectSuccess bool
		expectMessage string
		expectError   bool
	}{
		{
			name: "radius and contour installed",
			installState: helm.InstallState{
				RadiusInstalled:  true,
				RadiusVersion:    "v0.43.0",
				ContourInstalled: true,
				ContourVersion:   "v1.25.0",
			},
			expectSuccess: true,
			expectMessage: "Radius is installed (version: v0.43.0), Contour is installed (version: v1.25.0)",
		},
		{
			name: "radius installed but contour missing",
			installState: helm.InstallState{
				RadiusInstalled:  true,
				RadiusVersion:    "v0.43.0",
				ContourInstalled: false,
			},
			expectSuccess: true,
			expectMessage: "Radius is installed (version: v0.43.0), Contour is not installed (will be installed during upgrade)",
		},
		{
			name: "radius not installed",
			installState: helm.InstallState{
				RadiusInstalled: false,
			},
			expectSuccess: false,
			expectMessage: "Radius is not installed. Use 'rad install kubernetes' to install Radius first",
		},
		{
			name:        "helm error",
			helmError:   errors.New("failed to connect to cluster"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHelm := helm.NewMockInterface(ctrl)

			mockHelm.EXPECT().
				CheckRadiusInstall("test-context").
				Return(tt.installState, tt.helmError)

			check := NewRadiusInstallationCheck(mockHelm, "test-context")

			success, message, err := check.Run(context.Background())

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "failed to check Radius installation")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectSuccess, success)
				assert.Contains(t, message, tt.expectMessage)
			}
		})
	}
}

func TestRadiusInstallationCheck_Properties(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelm := helm.NewMockInterface(ctrl)
	check := NewRadiusInstallationCheck(mockHelm, "test-context")

	assert.Equal(t, "Radius Installation", check.Name())
	assert.Equal(t, SeverityError, check.Severity())
}
