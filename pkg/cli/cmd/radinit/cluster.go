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

package radinit

import (
	"context"
	"sort"

	"github.com/project-radius/radius/pkg/cli/clierrors"
	"github.com/project-radius/radius/pkg/version"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	selectClusterPrompt = "Select the kubeconfig context to install Radius into"
)

func (r *Runner) enterClusterOptions(ctx context.Context, options *initOptions) error {
	var err error
	options.Cluster.Context, err = r.selectCluster(ctx)
	if err != nil {
		return err
	}

	state, err := r.HelmInterface.CheckRadiusInstall(options.Cluster.Context)
	if err != nil {
		return clierrors.MessageWithCause(err, "Unable to verify Radius installation.")
	}
	options.Cluster.Install = !state.Installed

	if state.Installed {
		options.Cluster.Install = false
		options.Cluster.Version = state.Version
	}

	if options.Cluster.Install {
		options.Cluster.Install = true
		options.Cluster.Version = version.Version() // This may not be the precise version we install for a pre-release.
		options.Cluster.Namespace = "radius-system"
	}

	return nil
}

func (r *Runner) selectCluster(ctx context.Context) (string, error) {
	kubeContextList, err := r.KubernetesInterface.GetKubeContext()
	if err != nil {
		return "", clierrors.MessageWithCause(err, "Failed to read Kubernetes config.")
	}

	// In dev mode we will just take the default kubecontext
	if r.Dev {
		return kubeContextList.CurrentContext, nil
	}

	choices := r.buildClusterList(kubeContextList)
	cluster, err := r.Prompter.GetListInput(choices, selectClusterPrompt)
	if err != nil {
		return "", err
	}

	return cluster, nil
}

func (r *Runner) buildClusterList(config *api.Config) []string {
	// Ensure current context is at the top as the default
	// otherwise, sort the contexts alphabetically
	others := []string{}
	for k := range config.Contexts {
		if k != config.CurrentContext {
			others = append(others, k)
		}
	}

	sort.Strings(others)

	// Ensure current context is at the top as the default
	choices := []string{config.CurrentContext}
	choices = append(choices, others...)

	return choices
}
