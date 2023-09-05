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
	"bytes"
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/version"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func Test_enterInitOptions(t *testing.T) {
	t.Run("no flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		k8s := kubernetes.NewMockInterface(ctrl)
		helm := helm.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter, KubernetesInterface: k8s, HelmInterface: helm, ConfigHolder: &framework.ConfigHolder{Config: viper.New()}}

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

	t.Run("--full", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		k8s := kubernetes.NewMockInterface(ctrl)
		helm := helm.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter, KubernetesInterface: k8s, HelmInterface: helm, Full: true, ConfigHolder: &framework.ConfigHolder{Config: viper.New()}}

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

	t.Run("existing-workspace", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		k8s := kubernetes.NewMockInterface(ctrl)
		helm := helm.NewMockInterface(ctrl)

		var yaml = `
workspaces:
  default: abc
  items:
    abc:
      connection:
        kind: kubernetes
        context: cool-beans
      scope: /a/b/c
      environment: /a/b/c/providers/Applications.Core/environments/ice-cold
`
		v, err := makeConfig(yaml)
		runner := Runner{Prompter: prompter, KubernetesInterface: k8s, HelmInterface: helm, Full: true, ConfigHolder: &framework.ConfigHolder{Config: v}}

		require.NoError(t, err)
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
			Name: "abc",
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

	t.Run("existing-workspace-with-default-as-an-entry", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		k8s := kubernetes.NewMockInterface(ctrl)
		helm := helm.NewMockInterface(ctrl)

		var yaml = `
workspaces:
  default: default
  items:
    abc:
      connection:
        kind: kubernetes
        context: cool-beans
      scope: /a/b/c
      environment: /a/b/c/providers/Applications.Core/environments/ice-cold
    default:
      connection:
        kind: kubernetes
        context: hot-beans
      scope: /d/e/f
      environment: /a/b/c/providers/Applications.Core/environments/hot-coffee
`
		v, err := makeConfig(yaml)
		runner := Runner{Prompter: prompter, KubernetesInterface: k8s, HelmInterface: helm, Full: true, ConfigHolder: &framework.ConfigHolder{Config: v}}

		require.NoError(t, err)
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
			Name: "default",
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

func makeConfig(yaml string) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigType("YAML")
	err := v.ReadConfig(bytes.NewBuffer([]byte(yaml)))
	if err != nil {
		return nil, err
	}

	return v, nil
}
