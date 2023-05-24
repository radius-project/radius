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
