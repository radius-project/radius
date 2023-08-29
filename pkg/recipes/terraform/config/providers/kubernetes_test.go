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

package providers

import (
	"testing"

	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

func TestKubernetesProvider_BuildConfig(t *testing.T) {
	expectedConfig := map[string]any{
		"config_path": clientcmd.RecommendedHomeFile,
	}

	p := &kubernetesProvider{}
	config, err := p.BuildConfig(testcontext.New(t), nil)
	require.NoError(t, err)
	require.Equal(t, expectedConfig, config)
}

func TestKubernetesProvider_BuildConfig_Error(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "testvalue")
	t.Setenv("KUBERNETES_SERVICE_PORT", "1111")

	p := &kubernetesProvider{}
	config, err := p.BuildConfig(testcontext.New(t), nil)
	require.Error(t, err)
	require.Nil(t, config)
}
