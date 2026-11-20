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
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/components/metrics/metricsservice"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/common"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/engine"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func TestChartMetricsProofPorts(t *testing.T) {
	t.Parallel()
	components := map[string]string{
		"applications-rp": "rp", "dynamic-rp": "dynamic-rp", "ucp": "ucp", "controller": "controller",
	}
	for _, mode := range []string{"off", "dryRun", "enforce"} {
		for _, port := range []int{9090, 9197} {
			t.Run(fmt.Sprintf("%s/port=%d", mode, port), func(t *testing.T) {
				t.Parallel()
				selected, err := loader.Load("../../../deploy/Chart")
				require.NoError(t, err)
				templates := []*common.File{}
				for _, file := range selected.Templates {
					include := strings.HasSuffix(file.Name, ".tpl")
					for _, directory := range components {
						include = include || file.Name == "templates/"+directory+"/configmaps.yaml" ||
							file.Name == "templates/"+directory+"/deployment.yaml"
					}
					if include {
						templates = append(templates, file)
					}
				}
				selected.Templates = templates
				values := copyValues(selected.Values)
				// Leave the default port unset so this also verifies chart defaults.
				if port != 9090 {
					require.NoError(t, unstructured.SetNestedField(values, int64(port), "global", "prometheus", "port"))
				}
				require.NoError(t, unstructured.SetNestedField(values, mode == "enforce", "global", "rbac", "enabled"))
				require.NoError(t, unstructured.SetNestedField(values, mode == "dryRun", "global", "rbac", "dryRun"))
				renderValues, err := valuesutil.ToRenderValues(selected, values,
					common.ReleaseOptions{Name: radiusReleaseName, Namespace: RadiusSystemNamespace, IsInstall: true}, common.DefaultCapabilities)
				require.NoError(t, err)
				rendered, err := engine.Render(selected, renderValues)
				require.NoError(t, err)
				for name, directory := range components {
					var config corev1.ConfigMap
					require.NoError(t, yaml.Unmarshal([]byte(rendered["radius/templates/"+directory+"/configmaps.yaml"]), &config))
					require.Len(t, config.Data, 1)
					var settings struct {
						Metrics metricsservice.Options `json:"metricsProvider"`
					}
					for _, raw := range config.Data {
						require.NoError(t, yaml.Unmarshal([]byte(raw), &settings))
					}
					require.NotNil(t, settings.Metrics.Prometheus)
					require.Equal(t, port, settings.Metrics.Prometheus.Port, name+" runtime listener")
					var workload appsv1.Deployment
					require.NoError(t, yaml.Unmarshal([]byte(rendered["radius/templates/"+directory+"/deployment.yaml"]), &workload))
					require.Equal(t, fmt.Sprint(port), workload.Spec.Template.Annotations["prometheus.io/port"], name+" scrape port")
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprint(port)))),
						workload.Spec.Template.Annotations["checksum/metrics-port"], name+" reloads the configured listener on upgrade")
				}
			})
		}
	}
}
