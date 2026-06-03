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
	// Act
	values, err := buildContourValues(ContourChartOptions{HostNetwork: true})
	if err != nil {
		t.Fatalf("buildContourValues returned error: %v", err)
	}

	// Assert
	envoy, ok := values["envoy"].(map[string]any)
	if !ok {
		t.Fatalf("expected envoy map[string]any in values, got %T", values["envoy"])
	}

	if hostNetwork := envoy["hostNetwork"]; hostNetwork != true {
		t.Errorf("expected hostNetwork=true, got %v", hostNetwork)
	}
	if dnsPolicy := envoy["dnsPolicy"]; dnsPolicy != "ClusterFirstWithHostNet" {
		t.Errorf("expected dnsPolicy=ClusterFirstWithHostNet, got %v", dnsPolicy)
	}

	containerPorts, _ := envoy["containerPorts"].(map[string]any)
	wantContainer := map[string]any{"http": 80, "https": 443}
	if !reflect.DeepEqual(containerPorts, wantContainer) {
		t.Errorf("containerPorts mismatch.\nexpected: %v\ngot:      %v", wantContainer, containerPorts)
	}

	service, _ := envoy["service"].(map[string]any)
	servicePorts, _ := service["ports"].(map[string]any)
	wantService := map[string]any{"http": 8080, "https": 8443}
	if !reflect.DeepEqual(servicePorts, wantService) {
		t.Errorf("service ports mismatch.\nexpected: %v\ngot:      %v", wantService, servicePorts)
	}
}

func TestBuildContourValues_HostNetworkDisabled_ReturnsEmpty(t *testing.T) {
	// Act
	values, err := buildContourValues(ContourChartOptions{HostNetwork: false})
	if err != nil {
		t.Fatalf("buildContourValues returned error: %v", err)
	}

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
