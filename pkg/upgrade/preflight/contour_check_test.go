/*
Copyright 2026 The Radius Authors.

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
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestContourUpgradeCheck_Properties(t *testing.T) {
	t.Parallel()

	check := NewContourUpgradeCheck(helm.NewMockInterface(gomock.NewController(t)), helm.NewDefaultClusterOptions(), "test-context")

	require.Equal(t, "Contour Upgrade", check.Name())
	require.Equal(t, SeverityError, check.Severity())
}

func TestContourUpgradeCheck_Run(t *testing.T) {
	t.Parallel()

	friendly := clierrors.Message("Radius stopped before changing the cluster because Contour 1.32.0 cannot use the Gateway API CRDs in this cluster.")
	tests := []struct {
		name        string
		upgrade     bool
		err         error
		wantPass    bool
		wantMessage string
	}{
		{
			name:        "Contour passes the checks",
			upgrade:     true,
			wantPass:    true,
			wantMessage: "Contour can be upgraded and can use the Gateway API CRDs in the cluster",
		},
		{
			name:        "Contour is not installed",
			wantPass:    true,
			wantMessage: "Contour is not installed or is disabled, so the upgrade does not change it",
		},
		{
			name:        "Contour fails the checks",
			err:         friendly,
			wantMessage: "Contour cannot be upgraded",
		},
		{
			name:        "the checks fail to run",
			err:         errors.New("failed to list CRDs"),
			wantMessage: "Contour cannot be upgraded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The check must use the options of the upgrade, including the Contour values and --reset-values.
			options := helm.PopulateDefaultClusterOptions(helm.CLIClusterOptions{
				Contour:     helm.ChartOptions{SetArgs: []string{"contour.extraArgs={--disable-feature=tlsroutes}"}},
				ResetValues: true,
			})
			mockHelm := helm.NewMockInterface(gomock.NewController(t))
			mockHelm.EXPECT().CheckContourUpgrade(gomock.Any(), options, "test-context").Return(tt.upgrade, tt.err).Times(1)

			pass, message, err := NewContourUpgradeCheck(mockHelm, options, "test-context").Run(t.Context())

			require.Equal(t, tt.wantPass, pass)
			require.Equal(t, tt.wantMessage, message)
			// The error is returned unchanged, so that a friendly error stays friendly.
			require.Equal(t, tt.err, err)
		})
	}
}
