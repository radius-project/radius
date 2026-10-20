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
	"sort"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/version"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	selectClusterPrompt = "Select the kubeconfig context to install Radius into"
)

// enterClusterOptions selects a kube context and records whether Radius needs to be installed on it.
func (r *Runner) enterClusterOptions(options *initOptions) error {
	clusterContext, err := selectCluster(r.KubernetesInterface, r.Prompter, r.Full)
	if err != nil {
		return err
	}

	state, err := r.HelmInterface.CheckRadiusInstall(clusterContext)
	if err != nil {
		return clierrors.MessageWithCause(err, "Unable to verify Radius installation.")
	}

	options.Cluster = clusterOptions{Context: clusterContext}

	if state.RadiusInstalled {
		options.Cluster.Install = false
		options.Cluster.Version = state.RadiusVersion
	} else {
		options.Cluster.Install = true
		options.Cluster.Version = version.Version()
		options.Cluster.Namespace = "radius-system"
	}

	return nil
}

// selectCluster selects a kube context. If full is false, the current context is used automatically.
func selectCluster(k8s kubernetes.Interface, prompter prompt.Interface, full bool) (string, error) {
	kubeContextList, err := k8s.GetKubeContext()
	if err != nil {
		return "", clierrors.MessageWithCause(err, "Failed to read Kubernetes config.")
	}

	if !full {
		return kubeContextList.CurrentContext, nil
	}

	choices := buildClusterList(kubeContextList)
	cluster, err := prompter.GetListInput(choices, selectClusterPrompt)
	if err != nil {
		return "", err
	}

	return cluster, nil
}

// buildClusterList builds a sorted list of cluster contexts with the current context first.
func buildClusterList(config *api.Config) []string {
	others := []string{}
	for k := range config.Contexts {
		if k != config.CurrentContext {
			others = append(others, k)
		}
	}

	sort.Strings(others)

	choices := []string{config.CurrentContext}
	choices = append(choices, others...)

	return choices
}
