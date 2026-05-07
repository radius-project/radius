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
	err := runner.enterClusterOptions(&options)
	require.NoError(t, err)
	require.Equal(t, "kind-kind", options.Cluster.Context)
	require.Equal(t, true, options.Cluster.Install)
}
