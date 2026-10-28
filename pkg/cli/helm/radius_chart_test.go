/*
Copyright 2026 The Radius Authors.

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

package helm

import (
	"io"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yamlv3 "go.yaml.in/yaml/v3"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/release"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestRadiusChart_FluxSelection(t *testing.T) {
	t.Parallel()
	render := func(t *testing.T, flux map[string]any) (hostoptions.ProviderConfig, appsv1.Deployment) {
		t.Helper()
		chart, err := loader.Load("../../../deploy/Chart")
		require.NoError(t, err)
		install := helm.NewInstall(&helm.Configuration{})
		install.DryRunStrategy = helm.DryRunClient
		install.ReleaseName, install.Namespace = "radius", "radius-system"
		values := map[string]any{}
		if flux != nil {
			values["controller"] = map[string]any{"flux": flux}
		}
		rendered, err := install.RunWithContext(t.Context(), chart, values)
		require.NoError(t, err)
		accessor, err := release.NewAccessor(rendered)
		require.NoError(t, err)
		decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(accessor.Manifest()), 4096)
		var config hostoptions.ProviderConfig
		var deployment appsv1.Deployment
		for {
			var object map[string]any
			err := decoder.Decode(&object)
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			if object == nil {
				continue
			}
			data, err := yamlv3.Marshal(object)
			require.NoError(t, err)
			var meta metav1.PartialObjectMetadata
			require.NoError(t, yaml.NewYAMLOrJSONDecoder(strings.NewReader(string(data)), 4096).Decode(&meta))
			if meta.Kind == "ConfigMap" && meta.Name == "controller-config" {
				var cm corev1.ConfigMap
				require.NoError(t, yaml.NewYAMLOrJSONDecoder(strings.NewReader(string(data)), 4096).Decode(&cm))
				cfg := yamlv3.NewDecoder(strings.NewReader(cm.Data["controller-config.yaml"]))
				cfg.KnownFields(true)
				require.NoError(t, cfg.Decode(&config))
				require.NoError(t, config.Controller.Flux.Validate())
			}
			if meta.Kind == "Deployment" && meta.Name == "controller" {
				require.NoError(t, yaml.NewYAMLOrJSONDecoder(strings.NewReader(string(data)), 4096).Decode(&deployment))
			}
		}
		require.Equal(t, appsv1.RecreateDeploymentStrategyType, deployment.Spec.Strategy.Type)
		require.Nil(t, deployment.Spec.Strategy.RollingUpdate)
		require.NotEmpty(t, deployment.Spec.Template.Annotations["checksum/flux-config"])
		return config, deployment
	}
	disabled, before := render(t, nil)
	require.False(t, disabled.Controller.Flux.Enabled)
	require.Empty(t, disabled.Controller.Flux.Sources)
	selected, after := render(t, map[string]any{
		"enabled": true,
		"sources": []any{map[string]any{"namespace": "flux-system", "name": "applications", "uid": "source-uid"}},
	})
	require.True(t, selected.Controller.Flux.Enabled)
	require.Equal(t, []hostoptions.FluxSourceSelection{{Namespace: "flux-system", Name: "applications", UID: "source-uid"}}, selected.Controller.Flux.Sources)
	require.NotEqual(t, before.Spec.Template.Annotations["checksum/flux-config"], after.Spec.Template.Annotations["checksum/flux-config"])
}

func TestRadiusChart_ControllerResources(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		isUpgrade bool
		values    map[string]any
	}{
		{name: "default install"},
		{
			name:      "upgrade with obsolete annotation protection enabled",
			isUpgrade: true,
			values: map[string]any{
				"rp": map[string]any{
					"security": map[string]any{
						"annotationProtection": map[string]any{
							"enabled":                   true,
							"allowedServiceAccountName": "controller",
						},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			chart, err := loader.Load("../../../deploy/Chart")
			require.NoError(t, err)

			// Use the same offline rendering path as helm template --include-crds.
			install := helm.NewInstall(&helm.Configuration{})
			install.DryRunStrategy = helm.DryRunClient
			install.IncludeCRDs = true
			install.ReleaseName = "radius"
			install.Namespace = "radius-system"
			install.IsUpgrade = tt.isUpgrade
			rendered, err := install.RunWithContext(t.Context(), chart, tt.values)
			require.NoError(t, err)
			accessor, err := release.NewAccessor(rendered)
			require.NoError(t, err)

			counts := map[string]int{}
			decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(accessor.Manifest()), 4096)
			for {
				var object metav1.PartialObjectMetadata
				err := decoder.Decode(&object)
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				counts[object.Kind+"/"+object.Name]++
			}

			for _, resource := range []struct {
				kind  string
				name  string
				count int
			}{
				{"CustomResourceDefinition", "deploymenttemplates.radapp.io", 1},
				{"CustomResourceDefinition", "deploymentresources.radapp.io", 1},
				{"CustomResourceDefinition", "recipes.radapp.io", 0},
				{"ValidatingAdmissionPolicy", "radius-rp-annotation-protection", 0},
				{"ValidatingAdmissionPolicyBinding", "radius-rp-annotation-protection-binding", 0},
			} {
				key := resource.kind + "/" + resource.name
				assert.Equal(t, resource.count, counts[key], "rendered count of %s", key)
			}
		})
	}
}
