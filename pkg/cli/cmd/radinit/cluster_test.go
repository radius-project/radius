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
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd/api"
)

func Test_enterClusterOptions(t *testing.T) {
	ctrl := gomock.NewController(t)
	prompter := prompt.NewMockInterface(ctrl)
	k8s := kubernetes.NewMockInterface(ctrl)
	helm := helm.NewMockInterface(ctrl)
	runner := Runner{Prompter: prompter, KubernetesInterface: k8s, HelmInterface: helm, Full: true}

	initGetKubeContextSuccess(k8s)
	initKubeContextWithKind(prompter)
	initHelmMockRadiusNotInstalled(helm)

	options := initOptions{}
	err := runner.enterClusterOptions(context.Background(), &options)
	require.NoError(t, err)
	require.Equal(t, "kind-kind", options.Cluster.Context)
	require.Equal(t, true, options.Cluster.Install)
}

func Test_selectCluster(t *testing.T) {
	ctrl := gomock.NewController(t)
	prompter := prompt.NewMockInterface(ctrl)
	k8s := kubernetes.NewMockInterface(ctrl)
	runner := Runner{Prompter: prompter, KubernetesInterface: k8s, Full: true}

	initGetKubeContextSuccess(k8s)
	initKubeContextWithKind(prompter)

	name, err := runner.selectCluster(context.Background())
	require.NoError(t, err)
	require.Equal(t, "kind-kind", name)
}

func Test_buildClusterList(t *testing.T) {
	config := &api.Config{
		CurrentContext: "c-test-cluster",
		Contexts: map[string]*api.Context{
			"b-test-cluster": {},
			"a-test-cluster": {},
			"c-test-cluster": {},
		},
	}
	runner := Runner{Full: true}

	names := runner.buildClusterList(config)
	require.Equal(t, []string{"c-test-cluster", "a-test-cluster", "b-test-cluster"}, names)
}
