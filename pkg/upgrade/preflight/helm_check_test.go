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
	"go.uber.org/mock/gomock"
)

func TestHelmConnectivityCheck_Properties(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelm := helm.NewMockInterface(ctrl)
	check := NewHelmConnectivityCheck(mockHelm, "test-context")

	assert.Equal(t, "Helm Connectivity", check.Name())
	assert.Equal(t, SeverityError, check.Severity())
}

func TestHelmConnectivityCheck_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setupMock     func(*helm.MockInterface)
		expectPass    bool
		expectMessage string
		expectError   bool
	}{
		{
			name: "successful connection with deployed radius and contour",
			setupMock: func(mockHelm *helm.MockInterface) {
				installState := helm.InstallState{
					RadiusInstalled:  true,
					RadiusVersion:    "v0.47.0",
					ContourInstalled: true,
					ContourVersion:   "v1.25.0",
				}
				mockHelm.EXPECT().CheckRadiusInstall("test-context").Return(installState, nil)
			},
			expectPass:    true,
			expectMessage: "Helm successfully connected to cluster and found Radius release (version: v0.47.0), Contour installed (version: v1.25.0)",
			expectError:   false,
		},
		{
			name: "successful connection with radius but no contour",
			setupMock: func(mockHelm *helm.MockInterface) {
				installState := helm.InstallState{
					RadiusInstalled:  true,
					RadiusVersion:    "v0.47.0",
					ContourInstalled: false,
					ContourVersion:   "",
				}
				mockHelm.EXPECT().CheckRadiusInstall("test-context").Return(installState, nil)
			},
			expectPass:    true,
			expectMessage: "Helm successfully connected to cluster and found Radius release (version: v0.47.0)",
			expectError:   false,
		},
		{
			name: "helm connection fails",
			setupMock: func(mockHelm *helm.MockInterface) {
				mockHelm.EXPECT().CheckRadiusInstall("test-context").Return(helm.InstallState{}, errors.New("connection failed"))
			},
			expectPass:    false,
			expectMessage: "Cannot connect to cluster via Helm",
			expectError:   true,
		},
		{
			name: "radius release not found",
			setupMock: func(mockHelm *helm.MockInterface) {
				installState := helm.InstallState{
					RadiusInstalled:  false,
					ContourInstalled: false,
				}
				mockHelm.EXPECT().CheckRadiusInstall("test-context").Return(installState, nil)
			},
			expectPass:    false,
			expectMessage: "Helm can connect to cluster but Radius release not found",
			expectError:   false,
		},
		{
			name: "contour installed but radius not found",
			setupMock: func(mockHelm *helm.MockInterface) {
				installState := helm.InstallState{
					RadiusInstalled:  false,
					RadiusVersion:    "",
					ContourInstalled: true,
					ContourVersion:   "v1.25.0",
				}
				mockHelm.EXPECT().CheckRadiusInstall("test-context").Return(installState, nil)
			},
			expectPass:    false,
			expectMessage: "Helm can connect to cluster but Radius release not found",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHelm := helm.NewMockInterface(ctrl)
			tt.setupMock(mockHelm)

			check := NewHelmConnectivityCheck(mockHelm, "test-context")
			pass, message, err := check.Run(context.Background())

			assert.Equal(t, tt.expectPass, pass)
			assert.Contains(t, message, tt.expectMessage)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
