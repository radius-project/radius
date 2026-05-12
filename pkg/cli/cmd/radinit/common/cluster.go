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

package common

import (
	"sort"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/version"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	SelectClusterPrompt = "Select the kubeconfig context to install Radius into"
)

// ClusterResult holds the results of gathering cluster options.
type ClusterResult struct {
	Install   bool
	Namespace string
	Context   string
	Version   string
}

// EnterClusterOptions gathers cluster options by selecting a kube context and checking Radius install state.
func EnterClusterOptions(k8s kubernetes.Interface, helmClient helm.Interface, prompter prompt.Interface, full bool) (ClusterResult, error) {
	clusterContext, err := SelectCluster(k8s, prompter, full)
	if err != nil {
		return ClusterResult{}, err
	}

	state, err := helmClient.CheckRadiusInstall(clusterContext)
	if err != nil {
		return ClusterResult{}, clierrors.MessageWithCause(err, "Unable to verify Radius installation.")
	}

	result := ClusterResult{Context: clusterContext}

	if state.RadiusInstalled {
		result.Install = false
		result.Version = state.RadiusVersion
	} else {
		result.Install = true
		result.Version = version.Version()
		result.Namespace = "radius-system"
	}

	return result, nil
}

// SelectCluster selects a kube context. If full is false, the current context is used automatically.
func SelectCluster(k8s kubernetes.Interface, prompter prompt.Interface, full bool) (string, error) {
	kubeContextList, err := k8s.GetKubeContext()
	if err != nil {
		return "", clierrors.MessageWithCause(err, "Failed to read Kubernetes config.")
	}

	if !full {
		return kubeContextList.CurrentContext, nil
	}

	choices := BuildClusterList(kubeContextList)
	cluster, err := prompter.GetListInput(choices, SelectClusterPrompt)
	if err != nil {
		return "", err
	}

	return cluster, nil
}

// BuildClusterList builds a sorted list of cluster contexts with the current context first.
func BuildClusterList(config *api.Config) []string {
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
