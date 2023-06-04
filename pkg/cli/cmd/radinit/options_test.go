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
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/version"
	"github.com/stretchr/testify/require"
)

func Test_enterInitOptions(t *testing.T) {
	t.Run("dev", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		k8s := kubernetes.NewMockInterface(ctrl)
		helm := helm.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter, KubernetesInterface: k8s, HelmInterface: helm, Dev: true}

		initGetKubeContextSuccess(k8s)
		initHelmMockRadiusNotInstalled(helm)
		setScaffoldApplicationPromptNo(prompter)

		options, workspace, err := runner.enterInitOptions(context.Background())
		require.NoError(t, err)

		expectedWorkspace := workspaces.Workspace{
			Name: "default",
			Connection: map[string]any{
				"context": "kind-kind",
				"kind":    workspaces.KindKubernetes,
			},
			Environment: "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default",
			Scope:       "/planes/radius/local/resourceGroups/default",
		}
		require.Equal(t, expectedWorkspace, *workspace)

		expectedOptions := initOptions{
			Cluster: clusterOptions{
				Context:   "kind-kind",
				Install:   true,
				Namespace: "radius-system",
				Version:   version.Version(),
			},
			Environment: environmentOptions{
				Create:    true,
				Name:      "default",
				Namespace: "default",
			},
			Recipes: recipePackOptions{
				DevRecipes: true,
			},
		}
		require.Equal(t, expectedOptions, *options)
	})

	t.Run("non-dev", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		k8s := kubernetes.NewMockInterface(ctrl)
		helm := helm.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter, KubernetesInterface: k8s, HelmInterface: helm}

		initGetKubeContextSuccess(k8s)
		initKubeContextWithKind(prompter)
		initHelmMockRadiusNotInstalled(helm)
		initEnvNamePrompt(prompter, "test-env")
		initNamespacePrompt(prompter, "test-namespace")
		initAddCloudProviderPromptNo(prompter)
		setScaffoldApplicationPromptNo(prompter)

		options, workspace, err := runner.enterInitOptions(context.Background())
		require.NoError(t, err)

		expectedWorkspace := workspaces.Workspace{
			Name: "test-env",
			Connection: map[string]any{
				"context": "kind-kind",
				"kind":    workspaces.KindKubernetes,
			},
			Environment: "/planes/radius/local/resourceGroups/test-env/providers/Applications.Core/environments/test-env",
			Scope:       "/planes/radius/local/resourceGroups/test-env",
		}
		require.Equal(t, expectedWorkspace, *workspace)

		expectedOptions := initOptions{
			Cluster: clusterOptions{
				Context:   "kind-kind",
				Install:   true,
				Namespace: "radius-system",
				Version:   version.Version(),
			},
			Environment: environmentOptions{
				Create:    true,
				Name:      "test-env",
				Namespace: "test-namespace",
			},
		}
		require.Equal(t, expectedOptions, *options)
	})
}
