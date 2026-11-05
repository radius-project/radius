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
	"context"

	"github.com/radius-project/radius/pkg/cli/helm"
)

// Ensure ContourUpgradeCheck implements PreflightCheck interface
var _ PreflightCheck = (*ContourUpgradeCheck)(nil)

// ContourUpgradeCheck runs the Contour checks of the upgrade without changing the cluster. They verify that the
// Contour upgrade neither deletes CRDs nor takes over Gateway API CRDs that belong to something else, and that Contour
// can use the Gateway API CRDs in the cluster afterwards.
type ContourUpgradeCheck struct {
	helmInterface  helm.Interface
	clusterOptions helm.ClusterOptions
	kubeContext    string
}

// NewContourUpgradeCheck creates a new Contour upgrade check. clusterOptions must be the options of the upgrade, so
// that the check and the upgrade agree.
func NewContourUpgradeCheck(helmInterface helm.Interface, clusterOptions helm.ClusterOptions, kubeContext string) *ContourUpgradeCheck {
	return &ContourUpgradeCheck{
		helmInterface:  helmInterface,
		clusterOptions: clusterOptions,
		kubeContext:    kubeContext,
	}
}

// Name returns the name of this check.
func (c *ContourUpgradeCheck) Name() string {
	return "Contour Upgrade"
}

// Severity returns the severity level of this check.
func (c *ContourUpgradeCheck) Severity() CheckSeverity {
	return SeverityError
}

// Run executes the Contour upgrade check. It returns the error of the Contour checks unchanged, so that the CLI shows
// a friendly error as is.
func (c *ContourUpgradeCheck) Run(ctx context.Context) (bool, string, error) {
	upgrade, err := c.helmInterface.CheckContourUpgrade(ctx, c.clusterOptions, c.kubeContext)
	if err != nil {
		return false, "Contour cannot be upgraded", err
	}

	if !upgrade {
		return true, "Contour is not installed or is disabled, so the upgrade does not change it", nil
	}

	return true, "Contour can be upgraded and can use the Gateway API CRDs in the cluster", nil
}
