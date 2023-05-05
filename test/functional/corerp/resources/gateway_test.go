// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
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
	template := "testdata/gateways/corerp-resources-gateway.bicep"
	name := "corerp-resources-gateway"
	appNamespace := "default-corerp-resources-gateway"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
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
						validation.NewK8sPodForResource(name, "http-gtwy-front-ctnr"),
						validation.NewK8sPodForResource(name, "http-gtwy-back-ctnr"),
						validation.NewK8sHTTPProxyForResource(name, "http-gtwy-gtwy"),
						validation.NewK8sHTTPProxyForResource(name, "http-gtwy-front-rte"),
						validation.NewK8sServiceForResource(name, "http-gtwy-front-rte"),
						validation.NewK8sHTTPProxyForResource(name, "http-gtwy-back-rte"),
						validation.NewK8sServiceForResource(name, "http-gtwy-back-rte"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct corerp.CoreRPTest) {
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

func Test_Gateway_SSLPassthrough(t *testing.T) {
	template := "testdata/gateways/corerp-resources-gateway-sslpassthrough.bicep"
	name := "corerp-resources-gateway-sslpassthrough"
	appNamespace := "default-corerp-resources-gateway-sslpassthrough"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), "@testdata/parameters/test-tls-cert.parameters.json"),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
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
						validation.NewK8sPodForResource(name, "ssl-gtwy-front-ctnr"),
						validation.NewK8sHTTPProxyForResource(name, "ssl-gtwy-gtwy"),
						validation.NewK8sHTTPProxyForResource(name, "ssl-gtwy-front-rte"),
						validation.NewK8sServiceForResource(name, "ssl-gtwy-front-rte"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct corerp.CoreRPTest) {
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
	template := "testdata/gateways/corerp-resources-gateway-tlstermination.bicep"
	name := "corerp-resources-gateway-tlstermination"
	appNamespace := "default-corerp-resources-gateway-tlstermination"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), "@testdata/parameters/test-tls-cert.parameters.json"),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
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
						validation.NewK8sPodForResource(name, "tls-gtwy-front-ctnr"),
						validation.NewK8sHTTPProxyForResource(name, "tls-gtwy-gtwy"),
						validation.NewK8sHTTPProxyForResource(name, "tls-gtwy-front-rte"),
						validation.NewK8sServiceForResource(name, "tls-gtwy-front-rte"),
						validation.NewK8sSecretForResource(name, "tls-gtwy-cert"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct corerp.CoreRPTest) {
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

func testGatewayWithPortForward(t *testing.T, ctx context.Context, at corerp.CoreRPTest, hostname string, remotePort int, isHttps bool, tests []GatewayTestConfig) error {
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
