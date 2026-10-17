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
	"testing"

	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"k8s.io/client-go/tools/clientcmd/api"
)

func Test_SelectCluster(t *testing.T) {
	t.Run("full mode prompts user", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		k8s := kubernetes.NewMockInterface(ctrl)
		prompter := prompt.NewMockInterface(ctrl)

		k8s.EXPECT().GetKubeContext().Return(getTestKubeConfig(), nil).Times(1)
		prompter.EXPECT().GetListInput(gomock.Any(), selectClusterPrompt).Return("kind-kind", nil).Times(1)

		name, err := selectCluster(k8s, prompter, true)
		require.NoError(t, err)
		require.Equal(t, "kind-kind", name)
	})

	t.Run("non-full mode uses current context", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		k8s := kubernetes.NewMockInterface(ctrl)
		prompter := prompt.NewMockInterface(ctrl)

		k8s.EXPECT().GetKubeContext().Return(getTestKubeConfig(), nil).Times(1)

		name, err := selectCluster(k8s, prompter, false)
		require.NoError(t, err)
		require.Equal(t, "kind-kind", name)
	})
}

func Test_BuildClusterList(t *testing.T) {
	config := &api.Config{
		CurrentContext: "c-test-cluster",
		Contexts: map[string]*api.Context{
			"b-test-cluster": {},
			"a-test-cluster": {},
			"c-test-cluster": {},
		},
	}

	names := buildClusterList(config)
	require.Equal(t, []string{"c-test-cluster", "a-test-cluster", "b-test-cluster"}, names)
}

func Test_enterClusterOptions(t *testing.T) {
	t.Run("--full - selected cluster without Radius", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		k8s := kubernetes.NewMockInterface(ctrl)
		helm := helm.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter, KubernetesInterface: k8s, HelmInterface: helm, Full: true}

		initGetKubeContextSuccess(k8s)
		initKubeContextWithKind(prompter)
		initHelmMockRadiusNotInstalled(helm)

		options := initOptions{}
		err := runner.enterClusterOptions(&options)
		require.NoError(t, err)
		require.Equal(t, "kind-kind", options.Cluster.Context)
		require.Equal(t, true, options.Cluster.Install)
	})

	t.Run("radius installed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		k8s := kubernetes.NewMockInterface(ctrl)
		helmMock := helm.NewMockInterface(ctrl)
		prompter := prompt.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter, KubernetesInterface: k8s, HelmInterface: helmMock}

		k8s.EXPECT().GetKubeContext().Return(getTestKubeConfig(), nil).Times(1)
		helmMock.EXPECT().CheckRadiusInstall(gomock.Any()).Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "0.40"}, nil).Times(1)

		options := initOptions{}
		err := runner.enterClusterOptions(&options)
		require.NoError(t, err)
		require.False(t, options.Cluster.Install)
		require.Equal(t, "0.40", options.Cluster.Version)
		require.Equal(t, "kind-kind", options.Cluster.Context)
	})

	t.Run("radius not installed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		k8s := kubernetes.NewMockInterface(ctrl)
		helmMock := helm.NewMockInterface(ctrl)
		prompter := prompt.NewMockInterface(ctrl)
		runner := Runner{Prompter: prompter, KubernetesInterface: k8s, HelmInterface: helmMock}

		k8s.EXPECT().GetKubeContext().Return(getTestKubeConfig(), nil).Times(1)
		helmMock.EXPECT().CheckRadiusInstall(gomock.Any()).Return(helm.InstallState{RadiusInstalled: false}, nil).Times(1)

		options := initOptions{}
		err := runner.enterClusterOptions(&options)
		require.NoError(t, err)
		require.True(t, options.Cluster.Install)
		require.Equal(t, "radius-system", options.Cluster.Namespace)
		require.Equal(t, "kind-kind", options.Cluster.Context)
	})
}
