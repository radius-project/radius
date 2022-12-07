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
	"testing"
	"time"

	"github.com/Azure/go-autorest/autorest"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

const (
	retries      = 3
	remotePort   = 8080
	retryTimeout = 1 * time.Minute
	retryBackoff = 1 * time.Second
)

func Test_Gateway(t *testing.T) {
	template := "testdata/corerp-resources-gateway.bicep"
	name := "corerp-resources-gateway"

	requiredSecrets := map[string]map[string]string{}

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
					"default": {
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
				// Get hostname from root HTTPProxy in 'default' namespace
				hostname, err := functional.GetHostnameForHTTPProxy(ctx, ct.Options.Client, "default", name)
				require.NoError(t, err)
				t.Logf("found root proxy with hostname: {%s}", hostname)

				// Set up pod port-forwarding for contour-envoy
				t.Logf("Setting up portforward")
				// TODO: simplify code logic complexity through - https://github.com/project-radius/radius/issues/4778
				err = testGatewayWithPortForward(t, ctx, ct, hostname, remotePort, false)
				if err != nil {
					t.Logf("Failed to test Gateway via portforward with error: %s", err)
				} else {
					// Successfully ran tests
					return
				}

				require.Fail(t, "Gateway tests failed")
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func testGatewayWithPortForward(t *testing.T, ctx context.Context, at corerp.CoreRPTest, hostname string, remotePort int, isHttps bool) error {
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
		baseURL := fmt.Sprintf("http://localhost:%d", localPort)

		t.Logf("Portforward session active at %s", baseURL)

		if isHttps {
			if err := testGatewayAvailability(t, hostname, baseURL, "", 404, true); err != nil {
				close(stopChan)
				return err
			}
			return nil
		}

		if err := testGatewayAvailability(t, hostname, baseURL, "healthz", 200, false); err != nil {
			close(stopChan)
			return err
		}

		// Both of these URLs route to the same backend service,
		// but /backend2 maps to / which allows it to access /healthz
		if err := testGatewayAvailability(t, hostname, baseURL, "backend2/healthz", 200, false); err != nil {
			close(stopChan)
			return err
		}

		if err := testGatewayAvailability(t, hostname, baseURL, "backend1/healthz", 404, false); err != nil {
			close(stopChan)
			return err
		}

		// All of the requests were successful
		t.Logf("All requests encountered the correct status code")
		return nil
	}
}

func Test_HTTPSGateway(t *testing.T) {
	template := "testdata/corerp-resources-secure-gateway.bicep"
	name := "corerp-resources-gateways"

	requiredSecrets := map[string]map[string]string{}

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
						Name: "gtwy-gtwy",
						Type: validation.GatewaysResource,
						App:  name,
					},
					{
						Name: "gtwy-front-rte",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "gtwy-front-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "gtwy-front-ctnr"),
						validation.NewK8sHTTPProxyForResource(name, "gtwy-gtwy"),
						validation.NewK8sHTTPProxyForResource(name, "gtwy-front-rte"),
						validation.NewK8sServiceForResource(name, "gtwy-front-rte"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, ct corerp.CoreRPTest) {
				// Get hostname from root HTTPProxy in 'default' namespace
				hostname, err := functional.GetHostnameForHTTPProxy(ctx, ct.Options.Client, "default", name)
				require.NoError(t, err)
				t.Logf("found root proxy with hostname: {%s}", hostname)

				// Set up pod port-forwarding for contour-envoy
				t.Logf("Setting up portforward")
				err = testGatewayWithPortForward(t, ctx, ct, hostname, remotePort, true)
				if err != nil {
					t.Logf("Failed to test Gateway via portforward with error: %s", err)
				} else {
					// Successfully ran tests
					return
				}

				require.Fail(t, "Gateway tests failed")
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func testGatewayAvailability(t *testing.T, hostname, baseURL, path string, expectedStatusCode int, isHttps bool) error {
	req, err := autorest.Prepare(&http.Request{},
		autorest.WithBaseURL(baseURL),
		autorest.WithPath(path))
	if err != nil {
		return err
	}

	if !isHttps {
		req.Host = hostname
	}

	// Send requests to backing container via port-forward
	response, err := autorest.SendWithSender(
		newTestHTTPClient(isHttps, hostname),
		req,
		autorest.WithLogging(functional.NewTestLogger(t)),
		autorest.DoErrorUnlessStatusCode(expectedStatusCode),
		autorest.DoRetryForDuration(retryTimeout, retryBackoff))

	if response.Body != nil {
		defer response.Body.Close()
	}

	if err != nil {
		return err
	}

	// Encountered the correct status code
	return nil
}

func newTestHTTPClient(isHttps bool, hostname string) autorest.Sender {
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
	return &http.Client{Transport: transport}
}
