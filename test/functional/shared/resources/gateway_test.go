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
	retries         = 3
	httpRemotePort  = 8080
	httpsRemotePort = 8443
	retryTimeout    = 1 * time.Minute
	retryBackoff    = 1 * time.Second
)

// GatewayTestConfig is a struct that contains the configuration for a Gateway test
type GatewayTestConfig struct {
	Path               string
	ExpectedStatusCode int
}

func Test_Gateway(t *testing.T) {
	template := "testdata/corerp-resources-gateway.bicep"
	name := "corerp-resources-gateway"
	appNamespace := "default-corerp-resources-gateway"

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
						Name: "http-gtwy-gtwy",
						Type: validation.GatewaysResource,
						App:  name,
					},
					{
						Name: "http-gtwy-front-rte",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "http-gtwy-front-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "http-gtwy-back-rte",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "http-gtwy-back-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(validation.SourceRadius, "http-gtwy-front-ctnr",
							"Applications.Core/containers", name),
						validation.NewK8sHTTPProxyForResource(validation.SourceRadius, "http-gtwy-front-rte",
							"Applications.Core/httpRoutes", name),

						validation.NewK8sPodForResource(validation.SourceRadius, "http-gtwy-back-ctnr",
							"Applications.Core/containers", name),
						validation.NewK8sHTTPProxyForResource(validation.SourceRadius, "http-gtwy-back-rte",
							"Applications.Core/httpRoutes", name),

						validation.NewK8sHTTPProxyForResource(validation.SourceRadius, "http-gtwy-gtwy",
							"Applications.Core/gateways", name),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct shared.RPTest) {
				// Get hostname from root HTTPProxy in application namespace
				metadata, err := functional.GetHTTPProxyMetadata(ctx, ct.Options.Client, appNamespace, name)
				require.NoError(t, err)
				t.Logf("found root proxy with hostname: {%s} and status: {%s}", metadata.Hostname, metadata.Status)

				require.Equal(t, "Valid HTTPProxy", metadata.Status)

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
						validation.NewK8sPodForResource(validation.SourceRadius, "frontendcontainerdns",
							"Applications.Core/containers", name),
						// In bicep file there is only the container. How would HTTPProxy be created?
						validation.NewK8sServiceForResource(validation.SourceRadius, "frontendcontainerdns",
							"Applications.Core/httpRoutes", name),
						// How would gateway be created? If the container has a specific property, then the gateway is created?
						validation.NewK8sHTTPProxyForResource(validation.SourceRadius, "frontendcontainerdns",
							"Applications.Core/gateways", name),

						validation.NewK8sPodForResource(validation.SourceRadius, "backendcontainerdns",
							"Applications.Core/containers", name),
						// In bicep file there is only the container. How would HTTPProxy be created?
						validation.NewK8sServiceForResource(validation.SourceRadius, "backendcontainerdns",
							"Applications.Core/httpRoutes", name),
						// How would gateway be created? If the container has a specific property, then the gateway is created?
						validation.NewK8sHTTPProxyForResource(validation.SourceRadius, "backendcontainerdns",
							"Applications.Core/gateways", name),

						validation.NewK8sHTTPProxyForResource(validation.SourceRadius, "http-gtwy-gtwy-dns",
							"Applications.Core/gateways", name),
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
						Name: "ssl-gtwy-front-rte",
						Type: validation.HttpRoutesResource,
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
						validation.NewK8sPodForResource(validation.SourceRadius, "ssl-gtwy-front-ctnr",
							"Applications.Core/containers", name),
						validation.NewK8sHTTPProxyForResource(validation.SourceRadius, "ssl-gtwy-front-rte",
							"Applications.Core/httpRoutes", name),
						// Would an HTTPRoute also create a gateway?
						validation.NewK8sServiceForResource(validation.SourceRadius, "ssl-gtwy-front-rte",
							"Applications.Core/gateways", name),

						validation.NewK8sHTTPProxyForResource(validation.SourceRadius, "ssl-gtwy-gtwy",
							"Applications.Core/gateways", name),
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
						Name: "tls-gtwy-front-rte",
						Type: validation.HttpRoutesResource,
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
						validation.NewK8sPodForResource(validation.SourceRadius, "tls-gtwy-front-ctnr",
							"Applications.Core/containers", name),
						validation.NewK8sHTTPProxyForResource(validation.SourceRadius, "tls-gtwy-gtwy",
							"Applications.Core/gateways", name),

						validation.NewK8sHTTPProxyForResource(validation.SourceRadius, "tls-gtwy-front-rte",
							"Applications.Core/httpRoutes", name),
						validation.NewK8sServiceForResource(validation.SourceRadius, "tls-gtwy-front-rte",
							"Applications.Core/gateways", name),

						validation.NewK8sSecretForResource(validation.SourceRadius, "tls-gtwy-cert",
							"Applications.Core/secretStores", name),
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
			if err := testGatewayAvailability(hostname, baseURL, test.Path, test.ExpectedStatusCode, isHttps); err != nil {
				close(stopChan)
				return err
			}
		}

		// All of the requests were successful
		t.Logf("All requests encountered the correct status code")
		return nil
	}
}

func testGatewayAvailability(hostname, baseURL, path string, expectedStatusCode int, isHttps bool) error {
	urlPath := strings.TrimSuffix(baseURL, "/") + "/" + strings.TrimPrefix(path, "/")
	req, err := http.NewRequest(http.MethodGet, urlPath, nil)
	if err != nil {
		return err
	}

	req.Host = hostname

	client := newTestHTTPClient(isHttps, hostname)
	res, err := client.Do(req)
	if err != nil {
		return err
	}

	if res.StatusCode != expectedStatusCode {
		return fmt.Errorf("expected status code %d, got %d", expectedStatusCode, res.StatusCode)
	}

	// Encountered the correct status code
	return nil
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
