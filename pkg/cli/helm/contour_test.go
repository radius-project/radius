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
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"helm.sh/helm/v3/pkg/chart"
)

func TestBuildContourValues_HostNetworkEnabled(t *testing.T) {
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
)

func TestAddContourValues_HostNetworkEnabled(t *testing.T) {
	// Arrange
	chartVals := map[string]any{
		"envoy": map[string]any{
			"hostNetwork":    false,
			"dnsPolicy":      "",
			"containerPorts": map[string]any{},
			"service": map[string]any{
				"ports": map[string]any{},
			},
		},
		"configInline": map[string]any{},
		"gatewayAPI":   map[string]any{},
	}
	testChart := &chart.Chart{Values: chartVals}
	opts := ContourChartOptions{HostNetwork: true}

	// Act
	values, err := buildContourValues(ContourChartOptions{HostNetwork: true})
	if err != nil {
		t.Fatalf("buildContourValues returned error: %v", err)
	}

	// Assert
	envoy := requireMap(t, testChart.Values, "envoy")

	if hostNetwork := envoy["hostNetwork"]; hostNetwork != true {
		t.Errorf("expected hostNetwork=true, got %v", hostNetwork)
	}
	if dnsPolicy := envoy["dnsPolicy"]; dnsPolicy != "ClusterFirstWithHostNet" {
		t.Errorf("expected dnsPolicy=ClusterFirstWithHostNet, got %v", dnsPolicy)
	}

	containerPorts := requireMap(t, envoy, "containerPorts")
	wantContainer := map[string]any{"http": 80, "https": 443}
	if !reflect.DeepEqual(containerPorts, wantContainer) {
		t.Errorf("containerPorts mismatch.\nexpected: %v\ngot:      %v", wantContainer, containerPorts)
	}

	service := requireMap(t, envoy, "service")
	servicePorts := requireMap(t, service, "ports")
	wantService := map[string]any{"http": 8080, "https": 8443}
	if !reflect.DeepEqual(servicePorts, wantService) {
		t.Errorf("service ports mismatch.\nexpected: %v\ngot:      %v", wantService, servicePorts)
	}

	assertDefaultGatewayRef(t, testChart.Values)
	assertGatewayAPIManageCRDs(t, testChart.Values)
}

func TestBuildContourValues_HostNetworkDisabled_ReturnsEmpty(t *testing.T) {
	// Act
	values, err := buildContourValues(ContourChartOptions{HostNetwork: false})
	if err != nil {
		t.Fatalf("buildContourValues returned error: %v", err)
	}
func TestAddContourValues_HostNetworkDisabled_ConfiguresDefaultGatewayRef(t *testing.T) {
	// Arrange
	original := map[string]any{
		"envoy": map[string]any{
			"containerPorts": map[string]any{"http": 3000, "https": 3443},
			"service": map[string]any{
				"ports": map[string]any{"http": 3000, "https": 3443},
			},
		},
		"configInline": map[string]any{},
		"gatewayAPI":   map[string]any{},
	}
	testChart := &chart.Chart{Values: cloneMap(original)}
	expectedEnvoy := map[string]any{
		"containerPorts": map[string]any{"http": 3000, "https": 3443},
		"service": map[string]any{
			"ports": map[string]any{"http": 3000, "https": 3443},
		},
	}
	opts := ContourChartOptions{HostNetwork: false}

	// Assert – no overrides should be returned so the chart's defaults are used as-is.
	if len(values) != 0 {
		t.Errorf("expected empty values map when HostNetwork is false, got: %v", values)
	}
}

func Test_prepareContourChart_LoadChartError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	mockHelmClient.EXPECT().LoadChart("bad-contour-chart").Return(nil, errors.New("chart not found")).Times(1)
	helmAction := NewHelmAction(mockHelmClient)

	options := ContourChartOptions{
		ChartOptions: ChartOptions{
			Namespace: "radius-system",
			ChartPath: "bad-contour-chart",
		},
	}

	_, _, _, err := prepareContourChart(helmAction, options, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load Helm chart")
	// Assert - host network chart values should be unchanged.
	if !reflect.DeepEqual(testChart.Values["envoy"], expectedEnvoy) {
		t.Errorf("expected envoy chart values to remain unchanged when HostNetwork is false")
	}

	assertDefaultGatewayRef(t, testChart.Values)
	assertGatewayAPIManageCRDs(t, testChart.Values)
}

func TestAddContourValues_MergesGatewayConfig(t *testing.T) {
	// Arrange
	testChart := &chart.Chart{
		Values: map[string]any{
			"envoy": map[string]any{
				"containerPorts": map[string]any{},
				"service": map[string]any{
					"ports": map[string]any{},
				},
			},
			"configInline": map[string]any{
				"gateway": map[string]any{
					"controllerName": "projectcontour.io/gateway-controller",
				},
			},
			"gatewayAPI": map[string]any{},
		},
	}
	opts := ContourChartOptions{HostNetwork: false}

	// Act
	err := addContourValues(testChart, opts)

	// Assert
	require.NoError(t, err)
	configInline := requireMap(t, testChart.Values, "configInline")
	gateway := requireMap(t, configInline, "gateway")
	require.Equal(t, "projectcontour.io/gateway-controller", gateway["controllerName"])
	assertDefaultGatewayRef(t, testChart.Values)
	assertGatewayAPIManageCRDs(t, testChart.Values)
}

func TestAddContourValues_HostNetworkEnabled_ReturnsErrorForInvalidEnvoyNode(t *testing.T) {
	// Arrange
	testChart := &chart.Chart{
		Values: map[string]any{
			"envoy":        "invalid",
			"configInline": map[string]any{},
			"gatewayAPI":   map[string]any{},
		},
	}
	opts := ContourChartOptions{HostNetwork: true}

	// Act
	err := addContourValues(testChart, opts)

	// Assert
	require.ErrorContains(t, err, "envoy node not found in chart values")
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
		ChartOptions: ChartOptions{
			Namespace: "radius-system",
			ChartPath: "test-contour-chart",
		},
		HostNetwork: true,
	}

	_, _, values, err := prepareContourChart(helmAction, options, "")
	require.NoError(t, err)

	// Verify that user values contain HostNetwork overrides
	envoy, ok := values["envoy"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, envoy["hostNetwork"])

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

func assertGatewayAPIManageCRDs(t *testing.T, values map[string]any) {
	t.Helper()

	gatewayAPI := requireMap(t, values, "gatewayAPI")
	if manageCRDs := gatewayAPI["manageCRDs"]; manageCRDs != true {
		t.Errorf("expected gatewayAPI.manageCRDs=true, got %v", manageCRDs)
	}
}

func requireMap(t *testing.T, values map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := values[key]
	require.Truef(t, ok, "expected %q to be present", key)

	typed, ok := value.(map[string]any)
	require.Truef(t, ok, "expected %q to be map[string]any, got %T", key, value)
	return typed
}
