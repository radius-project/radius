// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package setup

import (
	"context"

	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/version"
)

func Install(ctx context.Context, clusterOptions helm.ClusterOptions, kubeContext string) (bool, error) {
	step := output.BeginStep("Installing Radius version %s control plane...", version.Version())
	foundExisting, err := helm.InstallOnCluster(ctx, clusterOptions, kubeContext)
	if err != nil {
		return false, err
	}

	output.CompleteStep(step)
	return foundExisting, nil
}
