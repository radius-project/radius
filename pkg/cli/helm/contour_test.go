package helm

import (
	"reflect"
	"testing"

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
	}
	testChart := &chart.Chart{Values: chartVals}
	opts := ContourChartOptions{HostNetwork: true}

	// Act
	if err := addContourValues(testChart, opts); err != nil {
		t.Fatalf("addContourValues returned error: %v", err)
	}

	// Assert
	envoy := testChart.Values["envoy"].(map[string]any)

	if hostNetwork := envoy["hostNetwork"]; hostNetwork != true {
		t.Errorf("expected hostNetwork=true, got %v", hostNetwork)
	}
	if dnsPolicy := envoy["dnsPolicy"]; dnsPolicy != "ClusterFirstWithHostNet" {
		t.Errorf("expected dnsPolicy=ClusterFirstWithHostNet, got %v", dnsPolicy)
	}

	containerPorts := envoy["containerPorts"].(map[string]any)
	wantContainer := map[string]any{"http": 80, "https": 443}
	if !reflect.DeepEqual(containerPorts, wantContainer) {
		t.Errorf("containerPorts mismatch.\nexpected: %v\ngot:      %v", wantContainer, containerPorts)
	}

	service := envoy["service"].(map[string]any)
	servicePorts := service["ports"].(map[string]any)
	wantService := map[string]any{"http": 8080, "https": 8443}
	if !reflect.DeepEqual(servicePorts, wantService) {
		t.Errorf("service ports mismatch.\nexpected: %v\ngot:      %v", wantService, servicePorts)
	}
}

func TestAddContourValues_HostNetworkDisabled_NoChange(t *testing.T) {
	// Arrange
	original := map[string]any{
		"envoy": map[string]any{
			"containerPorts": map[string]any{"http": 3000, "https": 3443},
			"service": map[string]any{
				"ports": map[string]any{"http": 3000, "https": 3443},
			},
		},
	}
	testChart := &chart.Chart{Values: cloneMap(original)}
	opts := ContourChartOptions{HostNetwork: false}

	// Act
	if err := addContourValues(testChart, opts); err != nil {
		t.Fatalf("addContourValues returned error: %v", err)
	}

	// Assert – chart values should be unchanged.
	if !reflect.DeepEqual(testChart.Values, original) {
		t.Errorf("expected chart values to remain unchanged when HostNetwork is false")
	}
}

// cloneMap does a shallow copy of a map[string]any for test isolation.
func cloneMap(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
