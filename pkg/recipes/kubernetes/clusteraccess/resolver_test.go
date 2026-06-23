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
	"testing"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func Test_Resolve_PrefersInjectedKubeconfig(t *testing.T) {
	injectedCfg := &rest.Config{Host: "https://injected.example"}
	localCfg := &rest.Config{Host: "https://local.example"}

	r := &resolver{
		strategies: []clusterStrategy{
			&injectedKubeconfigStrategy{
				getenv:     func(string) string { return "/path/to/kubeconfig" },
				loadConfig: func(string) (*rest.Config, error) { return injectedCfg, nil },
			},
			&localStrategy{
				inClusterConfig: func() (*rest.Config, error) { return localCfg, nil },
			},
		},
	}

	got, err := r.Resolve(context.Background(), &recipes.Configuration{})
	require.NoError(t, err)
	require.Same(t, injectedCfg, got)
}

func Test_Resolve_FallsBackToLocalWhenNoInjectedKubeconfig(t *testing.T) {
	localCfg := &rest.Config{Host: "https://local.example"}

	r := &resolver{
		strategies: []clusterStrategy{
			&injectedKubeconfigStrategy{
				getenv: func(string) string { return "" },
			},
			&localStrategy{
				inClusterConfig: func() (*rest.Config, error) { return localCfg, nil },
			},
		},
	}

	got, err := r.Resolve(context.Background(), &recipes.Configuration{})
	require.NoError(t, err)
	require.Same(t, localCfg, got)
}

func Test_LocalStrategy_AlwaysApplies(t *testing.T) {
	s := &localStrategy{}
	require.True(t, s.appliesTo(nil))
	require.True(t, s.appliesTo(&recipes.Configuration{}))
}

func Test_LocalStrategy_UsesInClusterConfigWhenAvailable(t *testing.T) {
	inClusterCfg := &rest.Config{Host: "https://in-cluster.example"}
	s := &localStrategy{
		inClusterConfig: func() (*rest.Config, error) { return inClusterCfg, nil },
		localConfig: func() (*rest.Config, error) {
			t.Fatal("localConfig should not be called when in-cluster config is available")
			return nil, nil
		},
	}

	got, err := s.restConfig(context.Background(), &recipes.Configuration{})
	require.NoError(t, err)
	require.Same(t, inClusterCfg, got)
}

func Test_LocalStrategy_FallsBackToLocalKubeconfigWhenNotInCluster(t *testing.T) {
	localCfg := &rest.Config{Host: "https://local.example"}
	s := &localStrategy{
		inClusterConfig: func() (*rest.Config, error) { return nil, rest.ErrNotInCluster },
		localConfig:     func() (*rest.Config, error) { return localCfg, nil },
	}

	got, err := s.restConfig(context.Background(), &recipes.Configuration{})
	require.NoError(t, err)
	require.Same(t, localCfg, got)
}

func Test_LocalStrategy_PropagatesUnexpectedInClusterError(t *testing.T) {
	sentinel := errors.New("boom")
	s := &localStrategy{
		inClusterConfig: func() (*rest.Config, error) { return nil, sentinel },
		localConfig: func() (*rest.Config, error) {
			t.Fatal("localConfig should not be called on a non-ErrNotInCluster error")
			return nil, nil
		},
	}

	_, err := s.restConfig(context.Background(), &recipes.Configuration{})
	require.ErrorIs(t, err, sentinel)
}

func Test_InjectedKubeconfigStrategy_AppliesWhenEnvVarSet(t *testing.T) {
	set := &injectedKubeconfigStrategy{getenv: func(string) string { return "/path" }}
	require.True(t, set.appliesTo(&recipes.Configuration{}))

	unset := &injectedKubeconfigStrategy{getenv: func(string) string { return "" }}
	require.False(t, unset.appliesTo(&recipes.Configuration{}))
}

func Test_InjectedKubeconfigStrategy_LoadsConfigFromPath(t *testing.T) {
	want := &rest.Config{Host: "https://injected.example"}
	var loadedPath string
	s := &injectedKubeconfigStrategy{
		getenv: func(string) string { return "/etc/radius/target-kubeconfig/config" },
		loadConfig: func(path string) (*rest.Config, error) {
			loadedPath = path
			return want, nil
		},
	}

	got, err := s.restConfig(context.Background(), &recipes.Configuration{})
	require.NoError(t, err)
	require.Same(t, want, got)
	require.Equal(t, "/etc/radius/target-kubeconfig/config", loadedPath)
}

func Test_InjectedKubeconfigStrategy_ErrorsWhenKubeconfigUnreadable(t *testing.T) {
	s := &injectedKubeconfigStrategy{
		getenv:     func(string) string { return "/missing/kubeconfig" },
		loadConfig: func(string) (*rest.Config, error) { return nil, errors.New("no such file") },
	}

	_, err := s.restConfig(context.Background(), &recipes.Configuration{})
	require.Error(t, err)
	require.Contains(t, err.Error(), TargetKubeconfigEnvVar)
	require.Contains(t, err.Error(), "/missing/kubeconfig")
}

func Test_ResolveKubeconfigSource_PrefersInjectedKubeconfig(t *testing.T) {
	r := &resolver{
		strategies: []clusterStrategy{
			&injectedKubeconfigStrategy{
				getenv: func(string) string { return "/path/to/kubeconfig" },
			},
			&localStrategy{
				inClusterConfig: func() (*rest.Config, error) {
					t.Fatal("localStrategy should not be consulted when an injected kubeconfig is set")
					return nil, nil
				},
			},
		},
	}

	got, err := r.ResolveKubeconfigSource(context.Background(), &recipes.Configuration{})
	require.NoError(t, err)
	require.Equal(t, KubeconfigSource{Path: "/path/to/kubeconfig"}, got)
}

func Test_ResolveKubeconfigSource_FallsBackToLocal(t *testing.T) {
	r := &resolver{
		strategies: []clusterStrategy{
			&injectedKubeconfigStrategy{getenv: func(string) string { return "" }},
			&localStrategy{
				inClusterConfig: func() (*rest.Config, error) { return nil, rest.ErrNotInCluster },
			},
		},
	}

	got, err := r.ResolveKubeconfigSource(context.Background(), &recipes.Configuration{})
	require.NoError(t, err)
	require.Equal(t, KubeconfigSource{Path: clientcmd.RecommendedHomeFile}, got)
}

func Test_InjectedKubeconfigStrategy_KubeconfigSourceReturnsEnvPath(t *testing.T) {
	s := &injectedKubeconfigStrategy{getenv: func(string) string { return "/etc/radius/target-kubeconfig/config" }}

	got, err := s.kubeconfigSource(context.Background(), &recipes.Configuration{})
	require.NoError(t, err)
	require.Equal(t, KubeconfigSource{Path: "/etc/radius/target-kubeconfig/config"}, got)
}

func Test_LocalStrategy_KubeconfigSourceInClusterReturnsEmptyPath(t *testing.T) {
	s := &localStrategy{
		inClusterConfig: func() (*rest.Config, error) { return &rest.Config{}, nil },
	}

	got, err := s.kubeconfigSource(context.Background(), &recipes.Configuration{})
	require.NoError(t, err)
	require.Equal(t, KubeconfigSource{}, got)
}

func Test_LocalStrategy_KubeconfigSourceNotInClusterReturnsLocalKubeconfig(t *testing.T) {
	s := &localStrategy{
		inClusterConfig: func() (*rest.Config, error) { return nil, rest.ErrNotInCluster },
	}

	got, err := s.kubeconfigSource(context.Background(), &recipes.Configuration{})
	require.NoError(t, err)
	require.Equal(t, KubeconfigSource{Path: clientcmd.RecommendedHomeFile}, got)
}

func Test_LocalStrategy_KubeconfigSourcePropagatesUnexpectedError(t *testing.T) {
	sentinel := errors.New("boom")
	s := &localStrategy{
		inClusterConfig: func() (*rest.Config, error) { return nil, sentinel },
	}

	_, err := s.kubeconfigSource(context.Background(), &recipes.Configuration{})
	require.ErrorIs(t, err, sentinel)
}
