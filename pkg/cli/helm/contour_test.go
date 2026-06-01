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
	"reflect"
	"testing"
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
