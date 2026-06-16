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

package clusteraccess

import (
	"context"
	"errors"

	"github.com/radius-project/radius/pkg/recipes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var errNoStrategy = errors.New("no cluster access strategy applied")

// localStrategy resolves the control-plane cluster: the in-cluster config when
// Radius runs inside Kubernetes, falling back to the local kubeconfig file when
// it does not. This is the default strategy and preserves today's behavior.
//
// inClusterConfig and localConfig are fields so tests can substitute them; in
// production they are rest.InClusterConfig and a clientcmd-based loader.
type localStrategy struct {
	inClusterConfig func() (*rest.Config, error)
	localConfig     func() (*rest.Config, error)
}

var _ clusterStrategy = (*localStrategy)(nil)

// newLocalStrategy creates a localStrategy backed by the real Kubernetes config
// loaders.
func newLocalStrategy() *localStrategy {
	return &localStrategy{
		inClusterConfig: rest.InClusterConfig,
		localConfig: func() (*rest.Config, error) {
			return clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		},
	}
}

// appliesTo always returns true: localStrategy is the default fallback.
func (s *localStrategy) appliesTo(_ *recipes.Configuration) bool {
	return true
}

// restConfig returns the in-cluster config, or the local kubeconfig when not
// running in-cluster.
func (s *localStrategy) restConfig(_ context.Context, _ *recipes.Configuration) (*rest.Config, error) {
	config, err := s.inClusterConfig()
	if err == nil {
		return config, nil
	}

	if errors.Is(err, rest.ErrNotInCluster) {
		return s.localConfig()
	}

	return nil, err
}
