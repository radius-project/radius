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

package preview

import (
	"context"
	"path/filepath"

	"github.com/radius-project/radius/pkg/cli/cmd/radinit/common"
)

// confirmOptions shows a summary of the user's selections and prompts for confirmation.
func (r *Runner) confirmOptions(ctx context.Context, options *initOptions) (bool, error) {
	return common.ConfirmOptions(ctx, r.Prompter, toDisplayOptions(options))
}

// showProgress shows an updating progress display while the user's selections are being applied.
//
// This function should be called from a goroutine while installation proceeds in the background.
func (r *Runner) showProgress(ctx context.Context, options *initOptions, progressChan <-chan common.ProgressMsg) error {
	return common.ShowProgress(ctx, r.Prompter, toDisplayOptions(options), progressChan)
}

// toDisplayOptions converts the package-local initOptions into the common
// DisplayOptions consumed by the shared summary and progress views.
func toDisplayOptions(options *initOptions) common.DisplayOptions {
	recipePackLabel := ""
	if options.Recipes.DefaultRecipePack {
		recipePackLabel = "default recipe pack"
	}

	var scaffoldFiles []string
	if options.Application.Scaffold {
		scaffoldFiles = []string{"app.bicep", "bicepconfig.json", filepath.Join(".rad", "rad.yaml")}
	}

	return common.DisplayOptions{
		Cluster: common.ClusterDisplay{
			Install:   options.Cluster.Install,
			Namespace: options.Cluster.Namespace,
			Context:   options.Cluster.Context,
			Version:   options.Cluster.Version,
		},
		Environment: common.EnvironmentDisplay{
			Create:    options.Environment.Create,
			Name:      options.Environment.Name,
			Namespace: options.Environment.Namespace,
		},
		CloudProviders: common.CloudProvidersDisplay{
			Azure: options.CloudProviders.Azure,
			AWS:   options.CloudProviders.AWS,
		},
		Application: common.ApplicationDisplay{
			Scaffold:      options.Application.Scaffold,
			Name:          options.Application.Name,
			ScaffoldFiles: scaffoldFiles,
		},
		RecipePackLabel: recipePackLabel,
	}
}
