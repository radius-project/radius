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
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
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
	appNamespace := "default-corerp-resources-gateway-dns"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "http-gtwy-gtwy-dns",
						Type: validation.GatewaysResource,
						App:  name,
					},
					{
						Name: "frontendcontainerdns",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "backendcontainerdns",
						Type: validation.ContainersResource,
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
						validation.NewK8sHTTPProxyForResource(name, "http-gtwy-gtwy-dns"),
						validation.NewK8sHTTPProxyForResource(name, "frontendcontainerdns"),
						validation.NewK8sHTTPProxyForResource(name, "backendcontainerdns"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct shared.RPTest) {
				// Get hostname from root HTTPProxy in application namespace
				metadata, err := functional.GetHTTPProxyMetadata(ctx, ct.Options.Client, appNamespace, name)
				require.NoError(t, err)
				t.Logf("found root proxy with hostname: {%s} and status: {%s}", metadata.Hostname, metadata.Status)

				// Set up pod port-forwarding for contour-envoy
				t.Logf("Setting up portforward")

				err = testGatewayWithPortForward(t, ctx, ct, metadata.Hostname, httpRemotePort, false, []GatewayTestConfig{
					// /healthz is exposed on frontend container
					{
						Path:               "healthz",
						ExpectedStatusCode: http.StatusOK,
					},
					// /backend2 uses 'replacePrefix', so it can access /healthz on backend container
					{
						Path:               "backend2/healthz",
						ExpectedStatusCode: http.StatusOK,
					},
					// since /backend1/healthz is not exposed on frontend container, it should return 404
					{
						Path:               "backend1/healthz",
						ExpectedStatusCode: http.StatusNotFound,
					},
				})

				require.NoError(t, err)
			},
		},
	})

	test.Test(t)
}

func Test_Gateway_SSLPassthrough(t *testing.T) {
	template := "testdata/corerp-resources-gateway-sslpassthrough.bicep"
	name := "corerp-resources-gateway-sslpassthrough"
	appNamespace := "default-corerp-resources-gateway-sslpassthrough"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), "@testdata/parameters/test-tls-cert.parameters.json"),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "ssl-gtwy-gtwy",
						Type: validation.GatewaysResource,
						App:  name,
					},
					{
						Name: "ssl-gtwy-front-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "ssl-gtwy-front-ctnr"),
						validation.NewK8sHTTPProxyForResource(name, "ssl-gtwy-front-ctnr"),
						validation.NewK8sHTTPProxyForResource(name, "ssl-gtwy-gtwy"),
						validation.NewK8sServiceForResource(name, "ssl-gtwy-front-ctnr"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct shared.RPTest) {
				// Get hostname from root HTTPProxy in application namespace
				metadata, err := functional.GetHTTPProxyMetadata(ctx, ct.Options.Client, appNamespace, name)
				require.NoError(t, err)
				t.Logf("found root proxy with hostname: {%s} and status: {%s}", metadata.Hostname, metadata.Status)

				// Set up pod port-forwarding for contour-envoy
				t.Logf("Setting up portforward")
				err = testGatewayWithPortForward(t, ctx, ct, metadata.Hostname, httpsRemotePort, true, []GatewayTestConfig{
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

	test.Test(t)
}

func Test_Gateway_TLSTermination(t *testing.T) {
	template := "testdata/corerp-resources-gateway-tlstermination.bicep"
	name := "corerp-resources-gateway-tlstermination"
	appNamespace := "default-corerp-resources-gateway-tlstermination"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), "@testdata/parameters/test-tls-cert.parameters.json"),
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
			PostStepVerify: func(ctx context.Context, t *testing.T, ct shared.RPTest) {
				// Get hostname from root HTTPProxy in application namespace
				metadata, err := functional.GetHTTPProxyMetadata(ctx, ct.Options.Client, appNamespace, name)
				require.NoError(t, err)
				t.Logf("found root proxy with hostname: {%s} and status: {%s}", metadata.Hostname, metadata.Status)

				// Set up pod port-forwarding for contour-envoy
				t.Logf("Setting up portforward")
				err = testGatewayWithPortForward(t, ctx, ct, metadata.Hostname, httpsRemotePort, true, []GatewayTestConfig{
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

	test.Test(t)
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
					MessageContains: "invalid TLS certificate",
				},
			},
		},
	})

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor:                               step.NewDeployErrorExecutor(template, validateFn),
			SkipObjectValidation:                   true,
			SkipKubernetesOutputResourceValidation: true,
		},
	},
		unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      secret,
					"namespace": "mynamespace",
				},
				"type": "Opaque",
				"data": map[string]interface{}{
					"tls.crt": "",
					"tls.key": "",
				},
			},
		})

	test.Test(t)
}

func testGatewayWithPortForward(t *testing.T, ctx context.Context, at shared.RPTest, hostname string, remotePort int, isHttps bool, tests []GatewayTestConfig) error {
	// stopChan will close the port-forward connection on close
	stopChan := make(chan struct{})

	// portChan will be populated with the assigned port once the port-forward connection is opened on it
	portChan := make(chan int)

	// errorChan will contain any errors created from initializing the port-forwarding session
	errorChan := make(chan error)

	go functional.ExposeIngress(t, ctx, at.Options.K8sClient, at.Options.K8sConfig, remotePort, stopChan, portChan, errorChan)

	select {
	case err := <-errorChan:
		return fmt.Errorf("portforward failed with error: %s", err)
	case localPort := <-portChan:
		protocol := "http"
		if isHttps {
			protocol = "https"
		}
		baseURL := fmt.Sprintf("%s://localhost:%d", protocol, localPort)

		t.Logf("Portforward session active at %s", baseURL)

		for _, test := range tests {
			if err := testGatewayAvailability(t, hostname, baseURL, test.Path, test.ExpectedStatusCode, isHttps); err != nil {
				close(stopChan)
				return err
			}
		}

		// All of the requests were successful
		t.Logf("All requests encountered the correct status code")
		return nil
	}
}

func testGatewayAvailability(t *testing.T, hostname, baseURL, path string, expectedStatusCode int, isHttps bool) error {
	urlPath := strings.TrimSuffix(baseURL, "/") + "/" + strings.TrimPrefix(path, "/")
	req, err := http.NewRequest(http.MethodGet, urlPath, nil)
	if err != nil {
		return err
	}

	req.Host = hostname

	client := newTestHTTPClient(isHttps, hostname)

	retries := 2
	retryBackoff := 5 * time.Second
	for i := 1; i <= retries; i++ {
		res, err := client.Do(req)
		if err == nil && res.StatusCode == expectedStatusCode {
			// Got expected status code, return
			return nil
		}

		// If we got an error, or the status code was not what we expected, log the error and retry
		// Logging the request and response will help with debugging the issue

		t.Logf("failed to make request to %s with error: %s", urlPath, err)
		requestDump, err := httputil.DumpRequestOut(req, true)
		if err != nil {
			t.Logf("failed to dump request with error: %s", err)
		}
		t.Logf("request dump: %s", string(requestDump))

		if res == nil {
			t.Logf("response is nil")
		}

		if res != nil && res.StatusCode != expectedStatusCode {
			t.Logf("expected status code %d, got %d", expectedStatusCode, res.StatusCode)
			responseDump, err := httputil.DumpResponse(res, true)
			if err != nil {
				t.Logf("failed to dump response with error: %s", err)
			}
			t.Logf("response dump: %s", string(responseDump))
		}

		// Wait for retryBackoff before trying again
		time.Sleep(retryBackoff)
		continue
	}

	return fmt.Errorf("failed to make request to %s after %d retries", urlPath, retries)
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
