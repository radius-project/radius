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

package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
)

type TestOptions struct {
	ConfigFilePath string
	K8sClient      *k8s.Clientset
	K8sConfig      *rest.Config
	DynamicClient  dynamic.Interface
	Client         client.Client
}

func NewTestOptions(t *testing.T) TestOptions {
	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	contextName, err := kubernetes.GetContextFromConfigFileIfExists("", "")
	require.NoError(t, err, "failed to read k8s config")

	k8s, restConfig, err := kubernetes.NewClientset(contextName)
	require.NoError(t, err, "failed to create kubernetes client")

	dynamicClient, err := kubernetes.NewDynamicClient(contextName)
	require.NoError(t, err, "failed to create kubernetes dyamic client")

	client, err := kubernetes.NewRuntimeClient(contextName, kubernetes.Scheme)
	require.NoError(t, err, "failed to create runtime client")

	return TestOptions{
		ConfigFilePath: config.ConfigFileUsed(),
		K8sClient:      k8s,
		K8sConfig:      restConfig,
		Client:         client,
		DynamicClient:  dynamicClient,
	}
}
