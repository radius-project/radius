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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/release"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

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
