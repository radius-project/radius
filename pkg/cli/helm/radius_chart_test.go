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
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/release"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestRadiusChartInstallCapabilities(t *testing.T) {
	selected := certificateTestChart(t)
	groups := map[string]bool{}
	for _, file := range selected.CRDObjects() {
		decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(string(file.File.Data)), 4096)
		for {
			var crd apiextv1.CustomResourceDefinition
			err := decoder.Decode(&crd)
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			require.NotEmpty(t, crd.Spec.Group)
			groups[crd.Spec.Group] = true
		}
	}
	require.NotEmpty(t, groups)
	// Require literal capability arguments so every group can be compared.
	argument := regexp.MustCompile("^\\s*[\"`]([^\"`]+)[\"`]")
	for _, file := range selected.Templates {
		for _, tail := range strings.Split(string(file.Data), ".Capabilities.APIVersions.Has")[1:] {
			match := argument.FindStringSubmatch(tail)
			require.NotNil(t, match, "%s: capability checks must name a literal API", file.Name)
			group, _, _ := strings.Cut(match[1], "/")
			require.False(t, groups[group], "%s: install registers %s after preview", file.Name, group)
		}
	}
}

func TestExternalWorkloadCapabilities(t *testing.T) {
	selected := certificateTestChart(t)
	options := NewDefaultClusterOptions()
	values := map[string]any{}
	setValue(values, true, "global", "rbac", "enabled")
	setValue(values, "external", "global", "rbac", "tls", "provider")
	setValue(values, true, "dynamicrp", "buildkit", "enabled")
	_, settings, err := selectedCertificateValues(options, selected, values)
	require.NoError(t, err)
	for _, version := range []string{"v1.29.0", "v1.30.0"} {
		for _, vap := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/VAP=%t", version, vap), func(t *testing.T) {
				caps := capabilitiesWith()
				caps.KubeVersion.Version = version
				if vap {
					caps.APIVersions = append(caps.APIVersions, admissionPolicyAPI)
				}
				impl := &Impl{contourCluster: &fakeContourCluster{capabilities: caps}}
				manifest, err := impl.renderCertificateManifest(t.Context(), options, selected, values, "", nil, 0)
				if !vap {
					require.ErrorContains(t, err, "requires admissionregistration.k8s.io/v1/ValidatingAdmissionPolicy")
					return
				}
				require.NoError(t, err)
				require.Equal(t, version == "v1.29.0", strings.Contains(manifest, "container.apparmor.security.beta.kubernetes.io/buildkitd:"))
				require.Equal(t, version == "v1.30.0", strings.Contains(manifest, "appArmorProfile:"))
				require.NoError(t, checkExternalWorkloadManifest(manifest, settings))
			})
		}
	}
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
