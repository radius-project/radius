/*
Copyright 2023 The Radius Authors.

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

package resource_test

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	httpRemotePort  = 8080
	httpsRemotePort = 8443
)

// GatewayTestConfig is a struct that contains the configuration for a Gateway test
type GatewayTestConfig struct {
	Path               string
	ExpectedStatusCode int
}

func Test_GatewayDNS(t *testing.T) {
	template := "testdata/corerp-resources-gateway-dns.bicep"
	name := "corerp-resources-gateway-dns"
	appNamespace := name

	// The migrated Radius.Compute/routes HTTPRoute is created without hostnames, so it matches all hosts;
	// any Host header reaches the route via the shared managed Gateway. The value is not used for matching.
	hostname := "corerp-resources-gateway-dns.example.com"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "http-gtwy-gtwy-dns",
						Type: validation.ComputeRoutesResource,
						App:  name,
					},
					{
						Name: "frontendcontainerdns",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
					{
						Name: "backendcontainerdns",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "frontendcontainerdns"),
						validation.NewK8sPodForResource(name, "backendcontainerdns"),
						validation.NewK8sServiceForResource(name, "frontendcontainerdns"),
						validation.NewK8sServiceForResource(name, "backendcontainerdns"),
						validation.NewK8sHTTPRouteForResource(name, "http-gtwy-gtwy-dns"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				// Set up pod port-forwarding for the shared Gateway API envoy
				t.Logf("Setting up portforward")

				err := testGatewayWithPortForward(t, ctx, ct, appNamespace, hostname, httpRemotePort, false, []GatewayTestConfig{
					// /healthz is exposed on the frontend container, reached via the '/' rule
					{
						Path:               "healthz",
						ExpectedStatusCode: http.StatusOK,
					},
					// /backend1 routes to the backend container; Radius.Compute/routes has no path
					// rewrite, the backend receives '/backend1/healthz' (which it does not serve).
					{
						Path:               "backend1/healthz",
						ExpectedStatusCode: http.StatusNotFound,
					},
				})

				require.NoError(t, err)
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	runGatewayTest(t, test)
}

func Test_Gateway_SSLPassthrough(t *testing.T) {
	template := "testdata/corerp-resources-gateway-sslpassthrough.bicep"
	name := "corerp-resources-gateway-sslpassthrough"
	appNamespace := name

	// The TLSRoute matches on SNI; this hostname must equal the route's `hostnames` entry in the bicep and
	// is used as the TLS ServerName (SNI) by the test client.
	hostname := "ssl-gtwy.example.com"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "ssl-gtwy-gtwy",
						Type: validation.ComputeRoutesResource,
						App:  name,
					},
					{
						Name: "ssl-gtwy-front-ctnr",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "ssl-gtwy-front-ctnr"),
						validation.NewK8sTLSRouteForResource(name, "ssl-gtwy-gtwy"),
						validation.NewK8sServiceForResource(name, "ssl-gtwy-front-ctnr"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				// Set up pod port-forwarding for the shared Gateway API envoy
				t.Logf("Setting up portforward")
				err := testGatewayWithPortForward(t, ctx, ct, appNamespace, hostname, httpsRemotePort, true, []GatewayTestConfig{
					// /healthz is exposed on frontend container (which terminates TLS itself)
					{
						Path:               "healthz",
						ExpectedStatusCode: http.StatusOK,
					},
				})
				if err != nil {
					t.Logf("Failed to test Gateway via portforward with error: %s", err)
				} else {
					// Successfully ran tests
					return
				}

				require.Fail(t, "Gateway tests failed")
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), "@testdata/parameters/test-tls-cert.parameters.json", fmt.Sprintf("environment=%s", previewEnvID))

	runGatewayTest(t, test)
}

func Test_Gateway_Timeout(t *testing.T) {
	template := "testdata/corerp-resources-gateway-timeout.bicep"
	appName := "gateway-timeout-app"
	appNamespace := "default-gateway-timeout-app"
	gatewayName := "gateway-timeout"
	containerName := "gateway-timeout-ctnr"

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage(), "appName="+appName, "gatewayName="+gatewayName, "containerName="+containerName),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: appName,
						Type: validation.ApplicationsResource,
					},
					{
						Name: gatewayName,
						Type: validation.GatewaysResource,
						App:  appName,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(appName, containerName),
						validation.NewK8sHTTPProxyForResource(appName, gatewayName),
						validation.NewK8sHTTPProxyForResource(appName, containerName),
						validation.NewK8sServiceForResource(appName, containerName),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				// Get hostname from root HTTPProxy in application namespace
				metadata, err := testutil.GetHTTPProxyMetadata(ctx, ct.Options.Client, appNamespace, appName)
				require.NoError(t, err)
				t.Logf("found root proxy with hostname: {%s} and status: {%s}", metadata.Hostname, metadata.Status)

				// Set up pod port-forwarding for contour-envoy
				t.Logf("Setting up portforward")
				err = testGatewayWithPortForward(t, ctx, ct, appNamespace, metadata.Hostname, httpRemotePort, false, []GatewayTestConfig{
					// /healthz is exposed on frontend container
					{
						Path:               "healthz",
						ExpectedStatusCode: http.StatusOK,
					},
				})
				if err != nil {
					t.Logf("Failed to test Gateway via portforward with error: %s", err)
				} else {
					// Successfully ran tests
					return
				}

				require.Fail(t, "Gateway tests failed")
			},
		},
	})
	runGatewayTest(t, test)
}

func Test_Gateway_Timeout_Backend_Exceeds_Request(t *testing.T) {
	template := "testdata/corerp-resources-gateway-timeout-ber.bicep"
	appName := "gateway-timeout-ber-app"
	containerName := "gateway-timeout-ber-ctnr"
	gatewayName := "gateway-timeout-ber"

	validateFn := step.ValidateAnyDetails("DeploymentFailed", []step.DeploymentErrorDetail{
		{
			Code: "ResourceDeploymentFailure",
			Details: []step.DeploymentErrorDetail{
				{
					Code:            "BadRequest",
					MessageContains: "request timeout must be greater than or equal to backend request timeout",
				},
			},
		},
	})
	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			Executor:                               step.NewDeployErrorExecutor(template, validateFn, testutil.GetMagpieImage(), "appName="+appName, "gatewayName="+gatewayName, "containerName="+containerName),
			SkipObjectValidation:                   true,
			SkipKubernetesOutputResourceValidation: true,
		},
	})
	test.Test(t)
}

func Test_Gateway_Timeout_Invalid_Duration(t *testing.T) {
	template := "testdata/corerp-resources-gateway-timeout-invalid.bicep"
	appName := "gateway-timeout-invalid-app"
	containerName := "gateway-timeout-invalid-ctnr"
	gatewayName := "gateway-timeout-invalid"

	validateFn := step.ValidateAnyDetails("DeploymentFailed", []step.DeploymentErrorDetail{
		{
			Code: "HttpRequestPayloadAPISpecValidationFailed",
			Details: []step.DeploymentErrorDetail{
				{
					Code:            "InvalidProperties",
					MessageContains: "properties.routes.timeoutPolicy.request in body should match",
				},
			},
		},
	})

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			Executor:                               step.NewDeployErrorExecutor(template, validateFn, testutil.GetMagpieImage(), "appName="+appName, "gatewayName="+gatewayName, "containerName="+containerName),
			SkipObjectValidation:                   true,
			SkipKubernetesOutputResourceValidation: true,
		},
	})
	test.Test(t)
}

func Test_Gateway_TLSTermination(t *testing.T) {
	template := "testdata/corerp-resources-gateway-tlstermination.bicep"
	name := "corerp-resources-gateway-tlstermination"
	appNamespace := "default-corerp-resources-gateway-tlstermination"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage(), "@testdata/parameters/test-tls-cert.parameters.json"),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "tls-gtwy-gtwy",
						Type: validation.GatewaysResource,
						App:  name,
					},
					{
						Name: "tls-gtwy-cert",
						Type: validation.SecretStoresResource,
						App:  name,
					},
					{
						Name: "tls-gtwy-front-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "tls-gtwy-front-ctnr"),
						validation.NewK8sHTTPProxyForResource(name, "tls-gtwy-gtwy"),
						validation.NewK8sHTTPProxyForResource(name, "tls-gtwy-front-ctnr"),
						validation.NewK8sServiceForResource(name, "tls-gtwy-front-ctnr"),
						validation.NewK8sSecretForResource(name, "tls-gtwy-cert"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				// Get hostname from root HTTPProxy in application namespace
				metadata, err := testutil.GetHTTPProxyMetadata(ctx, ct.Options.Client, appNamespace, name)
				require.NoError(t, err)
				t.Logf("found root proxy with hostname: {%s} and status: {%s}", metadata.Hostname, metadata.Status)

				// Set up pod port-forwarding for contour-envoy
				t.Logf("Setting up portforward")
				err = testGatewayWithPortForward(t, ctx, ct, appNamespace, metadata.Hostname, httpsRemotePort, true, []GatewayTestConfig{
					// /healthz is exposed on frontend container
					{
						Path:               "healthz",
						ExpectedStatusCode: http.StatusOK,
					},
				})
				if err != nil {
					t.Logf("Failed to test Gateway via portforward with error: %s", err)
				} else {
					// Successfully ran tests
					return
				}

				require.Fail(t, "Gateway tests failed")
			},
		},
	})

	runGatewayTest(t, test)
}

func Test_Gateway_Failure(t *testing.T) {
	template := "testdata/corerp-resources-gateway-failure.bicep"
	name := "corerp-resources-gateway-failure"
	secret := "secret"

	// We might see either of these states depending on the timing.
	validateFn := step.ValidateAnyDetails("DeploymentFailed", []step.DeploymentErrorDetail{
		{
			Code: "ResourceDeploymentFailure",
			Details: []step.DeploymentErrorDetail{
				{
					Code:            "Internal",
					MessageContains: "Error - Type: TLSError",
				},
			},
		},
	})

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor:                               step.NewDeployErrorExecutor(template, validateFn),
			SkipObjectValidation:                   true,
			SkipKubernetesOutputResourceValidation: true,
		},
	},
		unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]any{
					"name":      secret,
					"namespace": "mynamespace",
				},
				"type": "Opaque",
				"data": map[string]any{
					"tls.crt": "",
					"tls.key": "",
				},
			},
		})

	test.Test(t)
}

// runGatewayTest prevents route reconciliation and probes from overlapping on the
// single Contour/Envoy dataplane used by the functional-test cluster.
func runGatewayTest(t *testing.T, test rp.RPTest) {
	t.Helper()
	test.RunSerial = true
	test.Test(t)
}

func testGatewayWithPortForward(t *testing.T, ctx context.Context, at rp.RPTest, namespace, hostname string, remotePort int, isHttps bool, tests []GatewayTestConfig) error {
	// stopChan will close the port-forward connection on close
	stopChan := make(chan struct{})

	// portChan will be populated with the assigned port once the port-forward connection is opened on it
	portChan := make(chan int)

	// errorChan receives both startup and runtime failures. Buffer it so ExposeIngress
	// can finish after this helper closes the session.
	errorChan := make(chan error, 1)

	go testutil.ExposeIngress(t, ctx, at.Options.K8sClient, at.Options.K8sConfig, remotePort, stopChan, portChan, errorChan)
	defer close(stopChan)

	select {
	case err := <-errorChan:
		if err == nil {
			return errors.New("portforward stopped before becoming ready")
		}
		return fmt.Errorf("portforward failed: %w", err)
	case localPort := <-portChan:
		protocol := "http"
		if isHttps {
			protocol = "https"
		}
		baseURL := fmt.Sprintf("%s://localhost:%d", protocol, localPort)

		t.Logf("Portforward session active at %s", baseURL)

		for _, test := range tests {
			if err := testGatewayAvailability(t, ctx, hostname, baseURL, test.Path, test.ExpectedStatusCode, isHttps, errorChan); err != nil {
				logGatewayNetworkDiagnostics(t, ctx, at, namespace)
				return err
			}
		}

		// All of the requests were successful
		t.Logf("All requests encountered the correct status code")
		return nil
	}
}

func testGatewayAvailability(t *testing.T, ctx context.Context, hostname, baseURL, path string, expectedStatusCode int, isHttps bool, portForwardErrors <-chan error) error {
	urlPath := strings.TrimSuffix(baseURL, "/") + "/" + strings.TrimPrefix(path, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath, nil)
	if err != nil {
		return err
	}

	req.Host = hostname

	client := newTestHTTPClient(isHttps, hostname)
	defer client.CloseIdleConnections()

	// A freshly-deployed gateway can briefly return errors (e.g. HTTP 503 from Envoy) while
	// Contour programs the xDS route/cluster for the new route. Poll over a realistic window
	// instead of failing after only a couple of attempts. See radius-project/radius#12298.
	retryBackoff := 5 * time.Second
	timeout := 90 * time.Second
	deadline := time.Now().Add(timeout)

	var lastRes *http.Response
	var lastErr error
	attempts := 0
	for {
		attempts++
		res, err := client.Do(req)
		if err == nil && res.StatusCode == expectedStatusCode {
			// Got expected status code, close the body and return.
			res.Body.Close()
			return nil
		}

		// Close the previously retained response body before overwriting it so we don't
		// accumulate open bodies/connections across the widened poll budget.
		if lastRes != nil {
			lastRes.Body.Close()
		}
		lastRes, lastErr = res, err

		// Log a concise message per attempt; the full request/response dump is emitted once
		// below when the poll budget is exhausted to avoid flooding the test log.
		if err != nil {
			t.Logf("attempt %d: failed to make request to %s with error: %s", attempts, urlPath, err)
		} else {
			t.Logf("attempt %d: expected status code %d, got %d from %s", attempts, expectedStatusCode, res.StatusCode, urlPath)
		}

		if time.Now().After(deadline) {
			break
		}

		select {
		case err := <-portForwardErrors:
			if lastRes != nil {
				lastRes.Body.Close()
			}
			if err == nil {
				return fmt.Errorf("portforward stopped while probing %s", urlPath)
			}
			return fmt.Errorf("portforward stopped while probing %s: %w", urlPath, err)
		case <-ctx.Done():
			if lastRes != nil {
				lastRes.Body.Close()
			}
			return fmt.Errorf("gateway probe canceled: %w", ctx.Err())
		case <-time.After(retryBackoff):
		}
	}

	// The poll budget is exhausted; dump the last request and response to help with debugging.
	requestDump, dumpErr := httputil.DumpRequestOut(req, true)
	if dumpErr != nil {
		t.Logf("failed to dump request with error: %s", dumpErr)
	}
	t.Logf("request dump: %s", string(requestDump))

	if lastRes == nil {
		t.Logf("last response is nil (last error: %v)", lastErr)
	} else {
		responseDump, dumpErr := httputil.DumpResponse(lastRes, true)
		if dumpErr != nil {
			t.Logf("failed to dump response with error: %s", dumpErr)
		}
		t.Logf("response dump: %s", string(responseDump))
		lastRes.Body.Close()
	}

	return fmt.Errorf("failed to make request to %s after %d attempts over %s", urlPath, attempts, timeout)
}

func logGatewayNetworkDiagnostics(t *testing.T, ctx context.Context, at rp.RPTest, namespace string) {
	t.Helper()
	t.Logf("Gateway network diagnostics for namespace %s", namespace)

	services, err := at.Options.K8sClient.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Logf("failed to list Services: %v", err)
	} else {
		for _, service := range services.Items {
			ports := make([]string, 0, len(service.Spec.Ports))
			for _, port := range service.Spec.Ports {
				ports = append(ports, fmt.Sprintf("%s:%d->%s/%s", port.Name, port.Port, port.TargetPort.String(), port.Protocol))
			}
			t.Logf("Service %s/%s: clusterIP=%s ports=%s selector=%v", service.Namespace, service.Name, service.Spec.ClusterIP, strings.Join(ports, ","), service.Spec.Selector)
		}
	}

	endpointSlices, err := at.Options.K8sClient.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Logf("failed to list EndpointSlices: %v", err)
		return
	}

	for _, endpointSlice := range endpointSlices.Items {
		ports := make([]string, 0, len(endpointSlice.Ports))
		for _, port := range endpointSlice.Ports {
			portName := "<unnamed>"
			if port.Name != nil {
				portName = *port.Name
			}
			portNumber := "<unknown>"
			if port.Port != nil {
				portNumber = strconv.FormatInt(int64(*port.Port), 10)
			}
			protocol := "<unknown>"
			if port.Protocol != nil {
				protocol = string(*port.Protocol)
			}
			ports = append(ports, fmt.Sprintf("%s:%s/%s", portName, portNumber, protocol))
		}

		endpoints := make([]string, 0, len(endpointSlice.Endpoints))
		for _, endpoint := range endpointSlice.Endpoints {
			endpoints = append(endpoints, fmt.Sprintf(
				"%s ready=%s serving=%s terminating=%s",
				strings.Join(endpoint.Addresses, ","),
				optionalBool(endpoint.Conditions.Ready),
				optionalBool(endpoint.Conditions.Serving),
				optionalBool(endpoint.Conditions.Terminating),
			))
		}

		t.Logf(
			"EndpointSlice %s/%s: service=%s ports=%s endpoints=[%s]",
			endpointSlice.Namespace,
			endpointSlice.Name,
			endpointSlice.Labels[discoveryv1.LabelServiceName],
			strings.Join(ports, ","),
			strings.Join(endpoints, "; "),
		)
	}
}

func optionalBool(value *bool) string {
	if value == nil {
		return "unknown"
	}
	return strconv.FormatBool(*value)
}

func newTestHTTPClient(isHttps bool, hostname string) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if isHttps {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, //ignore certificate verification errors; needed since we use self-signed cert for magpie
			MinVersion:         tls.VersionTLS12,
			ServerName:         hostname,
		}
	}

	return &http.Client{
		Transport: transport,
	}
}

func Test_TestGatewayAvailability_PortForwardStops(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	portForwardErrors := make(chan error, 1)
	portForwardErrors <- errors.New("tunnel closed")

	err := testGatewayAvailability(t, t.Context(), "localhost", server.URL, "healthz", http.StatusOK, false, portForwardErrors)
	require.ErrorContains(t, err, "portforward stopped while probing")
	require.ErrorContains(t, err, "tunnel closed")
}
