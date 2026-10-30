/*
Copyright 2025 The Radius Authors.

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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	chart "helm.sh/helm/v4/pkg/chart/v2"
)

func TestBuildContourValues_HostNetworkEnabled(t *testing.T) {
	// Act
	values, err := buildContourValues(ContourChartOptions{HostNetwork: true})
	require.NoError(t, err)

	// Assert - envoy host network overrides
	envoy := requireMap(t, values, "envoy")
	assert.Equal(t, true, envoy["hostNetwork"])
	assert.Equal(t, "ClusterFirstWithHostNet", envoy["dnsPolicy"])

	containerPorts := requireMap(t, envoy, "containerPorts")
	assert.Equal(t, 80, containerPorts["http"])
	assert.Equal(t, 443, containerPorts["https"])

	service := requireMap(t, envoy, "service")
	servicePorts := requireMap(t, service, "ports")
	assert.Equal(t, 8080, servicePorts["http"])
	assert.Equal(t, 8443, servicePorts["https"])

	// Assert - gateway config is always set
	assertDefaultGatewayRef(t, values)
	assertNoGatewayAPICRDManagement(t, values)
}

func TestBuildContourValues_HostNetworkDisabled(t *testing.T) {
	// Act
	values, err := buildContourValues(ContourChartOptions{HostNetwork: false})
	require.NoError(t, err)

	// Assert - no envoy overrides
	_, hasEnvoy := values["envoy"]
	assert.False(t, hasEnvoy, "expected no envoy key when HostNetwork is false")

	// Assert - gateway config is always set
	assertDefaultGatewayRef(t, values)
	assertNoGatewayAPICRDManagement(t, values)
}

// TestBuildContourValues_UserValues verifies that --contour-set and --contour-set-file values reach the Contour chart
// values and take precedence over the values Radius sets.
func TestBuildContourValues_UserValues(t *testing.T) {
	t.Parallel()

	setFile := filepath.Join(t.TempDir(), "annotations.yaml")
	require.NoError(t, os.WriteFile(setFile, []byte("from-file"), 0600))

	values, err := buildContourValues(ContourChartOptions{
		ChartOptions: ChartOptions{
			SetArgs: []string{
				"envoy.service.type=NodePort",
				"envoy.service.ports.http=9080",
				"gatewayAPI.manageCRDs=false",
				"contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes}",
				"contour.replicaCount=1",
				"contour.replicaCount=3",
			},
			SetFileArgs: []string{"envoy.service.annotations.example=" + setFile},
		},
		HostNetwork: true,
	})
	require.NoError(t, err)

	// User values win over the values Radius sets, and the last value for a key wins.
	assert.Equal(t, "NodePort", nestedValue(values, "envoy", "service", "type"))
	assert.Equal(t, int64(9080), nestedValue(values, "envoy", "service", "ports", "http"))
	assert.Equal(t, int64(3), nestedValue(values, "contour", "replicaCount"))
	assert.Equal(t, "from-file", nestedValue(values, "envoy", "service", "annotations", "example"))
	assert.Equal(t, []any{"--disable-feature=tlsroutes", "--disable-feature=tcproutes"}, nestedValue(values, "contour", "extraArgs"))

	// Values that the user does not override keep Radius's values.
	assertDefaultGatewayRef(t, values)
	assert.Equal(t, true, nestedValue(values, "envoy", "hostNetwork"))
	assert.Equal(t, 8443, nestedValue(values, "envoy", "service", "ports", "https"))

	// An explicit gatewayAPI.manageCRDs=false survives the default that Radius applies after inspecting the cluster.
	setDefaultValue(values, true, "gatewayAPI", "manageCRDs")
	assert.Equal(t, false, nestedValue(values, "gatewayAPI", "manageCRDs"))
}

func TestBuildContourValues_InvalidUserValues(t *testing.T) {
	t.Parallel()

	_, err := buildContourValues(ContourChartOptions{ChartOptions: ChartOptions{SetArgs: []string{"invalid_no_equals"}}})
	require.Error(t, err)

	_, err = buildContourValues(ContourChartOptions{ChartOptions: ChartOptions{SetFileArgs: []string{"key=./testdata/nonexistent-file.txt"}}})
	require.Error(t, err)
}

func Test_prepareContourChart_LoadChartError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	mockHelmClient.EXPECT().LoadChart("bad-contour-chart").Return(nil, errors.New("chart not found")).Times(1)
	helmAction := NewHelmAction(mockHelmClient)

	options := ContourChartOptions{
		Namespace: "radius-system",
		ChartPath: "bad-contour-chart",
	}

	_, _, _, err := prepareContourChart(t.Context(), helmAction, options, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load Helm chart")
}

func Test_prepareContourChart_DoesNotMutateChartValues(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	helmChart := &chart.Chart{
		Values: map[string]any{
			"envoy": map[string]any{
				"existing": "default-value",
			},
		},
	}
	mockHelmClient := NewMockHelmClient(ctrl)
	mockHelmClient.EXPECT().LoadChart("test-contour-chart").Return(helmChart, nil).Times(1)
	helmAction := NewHelmAction(mockHelmClient)

	options := ContourChartOptions{
		Namespace:   "radius-system",
		ChartPath:   "test-contour-chart",
		HostNetwork: true,
	}

	_, _, values, err := prepareContourChart(t.Context(), helmAction, options, "")
	require.NoError(t, err)

	// Verify that user values contain HostNetwork overrides
	envoy, ok := values["envoy"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, envoy["hostNetwork"])

	// Verify that user values contain gateway config
	assertDefaultGatewayRef(t, values)
	assertNoGatewayAPICRDManagement(t, values)

	// Verify that helmChart.Values remains unchanged
	chartEnvoy, ok := helmChart.Values["envoy"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "default-value", chartEnvoy["existing"])
	_, hasHostNetwork := chartEnvoy["hostNetwork"]
	assert.False(t, hasHostNetwork, "prepareContourChart must not mutate helmChart.Values")
}

func assertDefaultGatewayRef(t *testing.T, values map[string]any) {
	t.Helper()

	configInline := requireMap(t, values, "configInline")
	gateway := requireMap(t, configInline, "gateway")
	gatewayRef := requireMap(t, gateway, "gatewayRef")

	if name := gatewayRef["name"]; name != DefaultContourGatewayName {
		t.Errorf("expected gatewayRef.name=%s, got %v", DefaultContourGatewayName, name)
	}
	if namespace := gatewayRef["namespace"]; namespace != DefaultContourGatewayNamespace {
		t.Errorf("expected gatewayRef.namespace=%s, got %v", DefaultContourGatewayNamespace, namespace)
	}
}

// assertNoGatewayAPICRDManagement verifies that values do not decide whether the chart manages the Gateway API CRDs.
// The CRDs are shared by the whole cluster, so Radius decides only after it inspects the cluster.
func assertNoGatewayAPICRDManagement(t *testing.T, values map[string]any) {
	t.Helper()

	_, found := values["gatewayAPI"]
	assert.False(t, found, "gatewayAPI.manageCRDs depends on the cluster and must not be part of the static Contour values")
}

func requireMap(t *testing.T, values map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := values[key]
	require.Truef(t, ok, "expected %q to be present", key)

	typed, ok := value.(map[string]any)
	require.Truef(t, ok, "expected %q to be map[string]any, got %T", key, value)
	return typed
}
