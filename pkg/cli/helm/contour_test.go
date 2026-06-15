package helm

import (
	"maps"
	"reflect"
	"testing"

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
	if err := addContourValues(testChart, opts); err != nil {
		t.Fatalf("addContourValues returned error: %v", err)
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

	// Act
	if err := addContourValues(testChart, opts); err != nil {
		t.Fatalf("addContourValues returned error: %v", err)
	}

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

// cloneMap does a shallow copy of a map[string]any for test isolation.
func cloneMap(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	maps.Copy(out, src)
	return out
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
