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

func Install(ctx context.Context, clusterOptions helm.ClusterOptions, kubeContext string) (bool, error) {
	step := output.BeginStep("Installing Radius...")
	foundExisting, err := helm.InstallOnCluster(ctx, clusterOptions, kubeContext)
	if err != nil {
		return false, err
	}

	output.CompleteStep(step)
	return foundExisting, nil
}
