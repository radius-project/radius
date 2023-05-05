// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package setup

import (
	"context"

	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/output"
)

// # Function Explanation
// 
//	Uninstall is a function that uninstalls the Radius control plane from the cluster specified by the kubeContext 
//	parameter. It returns an error if the uninstallation fails, which should be handled by the caller.
func Uninstall(ctx context.Context, kubeContext string) error {
	step := output.BeginStep("Uninstalling Radius...")
	err := helm.UninstallOnCluster(kubeContext)
	if err != nil {
		return err
	}
	output.LogInfo("The Radius control plane was uninstalled successfully. Any existing environment metadata will be retained for future installations. Local workspaces are also retained. Use the rad workspace command if updates are needed to your local workspaces.")
	output.CompleteStep(step)
	return nil
}
