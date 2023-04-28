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
