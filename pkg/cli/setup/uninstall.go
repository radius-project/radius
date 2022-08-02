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
	output.LogInfo("Radius control plane uninstalled successfully. All the environment and managed application data created are still intact on server and config.yaml. Please use rad env or rad workspace commands if any updates are needed in config.yaml")
	output.CompleteStep(step)
	return nil
}
