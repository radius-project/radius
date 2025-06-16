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
	"fmt"

	"github.com/radius-project/radius/pkg/cli/helm"
)

// RadiusInstallationCheck validates that Radius is currently installed
// in the cluster and in a healthy state for upgrading.
type RadiusInstallationCheck struct {
	helmInterface helm.Interface
	kubeContext   string
}

// NewRadiusInstallationCheck creates a new Radius installation check.
func NewRadiusInstallationCheck(helmInterface helm.Interface, kubeContext string) *RadiusInstallationCheck {
	return &RadiusInstallationCheck{
		helmInterface: helmInterface,
		kubeContext:   kubeContext,
	}
}

// Name returns the name of this check.
func (r *RadiusInstallationCheck) Name() string {
	return "Radius Installation"
}

// Severity returns the severity level of this check.
func (r *RadiusInstallationCheck) Severity() CheckSeverity {
	return SeverityError
}

// Run executes the Radius installation check.
func (r *RadiusInstallationCheck) Run(ctx context.Context) (bool, string, error) {
	state, err := r.helmInterface.CheckRadiusInstall(r.kubeContext)
	if err != nil {
		return false, "", fmt.Errorf("failed to check Radius installation: %w", err)
	}

	if !state.RadiusInstalled {
		return false, "Radius is not installed. Use 'rad install kubernetes' to install Radius first", nil
	}

	message := fmt.Sprintf("Radius is installed (version: %s)", state.RadiusVersion)

	// Also check if Contour is installed
	if state.ContourInstalled {
		message += fmt.Sprintf(", Contour is installed (version: %s)", state.ContourVersion)
	} else {
		// This is a warning, not an error, as upgrade might install Contour
		message += ", Contour is not installed (will be installed during upgrade)"
	}

	return true, message, nil
}
