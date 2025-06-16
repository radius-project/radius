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

// HelmConnectivityCheck validates that Helm can connect to the cluster
// and access the Radius release for upgrade operations.
type HelmConnectivityCheck struct {
	helmInterface helm.Interface
	kubeContext   string
}

// NewHelmConnectivityCheck creates a new Helm connectivity check.
func NewHelmConnectivityCheck(helmInterface helm.Interface, kubeContext string) *HelmConnectivityCheck {
	return &HelmConnectivityCheck{
		helmInterface: helmInterface,
		kubeContext:   kubeContext,
	}
}

// Name returns the name of this check.
func (h *HelmConnectivityCheck) Name() string {
	return "Helm Connectivity"
}

// Severity returns the severity level of this check.
func (h *HelmConnectivityCheck) Severity() CheckSeverity {
	return SeverityError
}

// Run executes the Helm connectivity check.
func (h *HelmConnectivityCheck) Run(ctx context.Context) (bool, string, error) {
	installState, err := h.helmInterface.CheckRadiusInstall(h.kubeContext)
	if err != nil {
		return false, "Cannot connect to cluster via Helm", fmt.Errorf("failed to check Helm connectivity: %w", err)
	}

	if !installState.RadiusInstalled {
		return false, "Helm can connect to cluster but Radius release not found", nil
	}

	message := fmt.Sprintf("Helm successfully connected to cluster and found Radius release (version: %s)", installState.RadiusVersion)

	if installState.ContourInstalled {
		message += fmt.Sprintf(", Contour installed (version: %s)", installState.ContourVersion)
	}

	return true, message, nil
}
