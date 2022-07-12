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

	output.CompleteStep(step)
	return nil
}
