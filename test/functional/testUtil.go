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

package functional

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RadiusSystemNamespace = "radius-system"
)

func GetMagpieImage() string {
	defaultDockerReg, imageTag := setDefault()
	magpieImage := "magpieimage=" + defaultDockerReg + "/magpiego:" + imageTag
	return magpieImage
}

func GetMagpieTag() string {
	_, imageTag := setDefault()
	magpietag := "magpietag=" + imageTag
	return magpietag
}

func setDefault() (string, string) {
	defaultDockerReg := os.Getenv("DOCKER_REGISTRY")
	imageTag := os.Getenv("REL_VERSION")
	if defaultDockerReg == "" {
		defaultDockerReg = "radiusdev.azurecr.io"
	}
	if imageTag == "" {
		imageTag = "latest"
	}

	return defaultDockerReg, imageTag
}

type ProxyMetadata struct {
	Hostname string
	Status   string
}

func GetRecipeRegistry() string {
	defaultRecipeRegistry := os.Getenv("RECIPE_REGISTRY")
	if defaultRecipeRegistry == "" {
		defaultRecipeRegistry = "radiusdev.azurecr.io"
	}
	return "registry=" + defaultRecipeRegistry
}

func GetRecipeVersion() string {
	defaultVersion := os.Getenv("RECIPE_TAG_VERSION")
	if defaultVersion == "" {
		defaultVersion = "latest"
	}
	return "version=" + defaultVersion
}

// GetHTTPProxyMetadata finds the fqdn set on the root HTTPProxy of the specified application and the current status (e.g. "Valid", "Invalid")
func GetHTTPProxyMetadata(ctx context.Context, client runtime_client.Client, namespace, application string) (*ProxyMetadata, error) {
	httpproxies, err := GetHTTPProxyList(ctx, client, namespace, application)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve list of cluster HTTPProxies: %w", err)
	}

	for _, httpProxy := range httpproxies.Items {
		if httpProxy.Spec.VirtualHost != nil {
			// Found a root proxy
			return &ProxyMetadata{
				Hostname: httpProxy.Spec.VirtualHost.Fqdn,
				Status:   httpProxy.Status.Description,
			}, nil
		}
	}

	return nil, fmt.Errorf("could not find root proxy in list of cluster HTTPProxies")
}

// GetHTTPProxyList returns a list of HTTPProxies for the specified application
func GetHTTPProxyList(ctx context.Context, client runtime_client.Client, namespace, application string) (*contourv1.HTTPProxyList, error) {
	var httpproxies contourv1.HTTPProxyList

	label, err := labels.Parse(fmt.Sprintf("radius.dev/application=%s", application))
	if err != nil {
		return nil, err
	}

	err = client.List(ctx, &httpproxies, &runtime_client.ListOptions{
		Namespace:     namespace,
		LabelSelector: label,
	})
	if err != nil {
		return nil, err
	}

	return &httpproxies, nil
}

// ExposeIngress creates a port-forward session and sends the (assigned) local port to portChan
func ExposeIngress(t *testing.T, ctx context.Context, client *k8s.Clientset, config *rest.Config, remotePort int, stopChan chan struct{}, portChan chan int, errorChan chan error) {
	selector := "app.kubernetes.io/component=envoy"
	ExposePod(t, ctx, client, config, RadiusSystemNamespace, selector, remotePort, stopChan, portChan, errorChan)
}

// ExposePod creates a port-forward session and sends the (assigned) local port to portChan
func ExposePod(t *testing.T, ctx context.Context, client *k8s.Clientset, config *rest.Config, namespace string, selector string, remotePort int, stopChan chan struct{}, portChan chan int, errorChan chan error) {
	// Find matching pods
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector, Limit: 1})
	if err != nil {
		errorChan <- err
		return
	}

	if len(pods.Items) == 0 {
		errorChan <- fmt.Errorf("no pods exist for selector: %s", selector)
		return
	}

	pod := pods.Items[0]

	// Create API Server URL using pod name
	url := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(pod.Namespace).
		Name(pod.Name).
		SubResource("portforward").URL()

	transport, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		errorChan <- err
		return
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, url)

	tw := TestWriter{t}
	out, errOut := tw, tw

	readyChan := make(chan struct{})
	forwarder, err := portforward.New(dialer, []string{fmt.Sprintf(":%d", remotePort)}, stopChan, readyChan, out, errOut)
	if err != nil {
		errorChan <- err
		return
	}

	// Run the port-forward with the desired configuration
	go func() {
		errorChan <- forwarder.ForwardPorts()
	}()

	// Wait for the forwarder to be ready, then get the assigned port
	<-readyChan
	ports, err := forwarder.GetPorts()
	if err != nil {
		errorChan <- err
	}

	// Send the assigned port to then portChan channel
	portChan <- int(ports[0].Local)
}

func NewTestLogger(t *testing.T) *log.Logger {
	tw := TestWriter{t}
	logger := log.Logger{}
	logger.SetOutput(tw)

	return &logger
}

// IsMapSubSet returns true if the expectedMap is a subset of the actualMap
func IsMapSubSet(expectedMap map[string]string, actualMap map[string]string) bool {
	if len(expectedMap) > len(actualMap) {
		return false
	}

	for k1, v1 := range expectedMap {
		v2, ok := actualMap[k1]
		if !(ok && strings.EqualFold(v1, v2)) {
			return false
		}

	}

	return true
}

// IsMapNonIntersecting returns true if the notExpectedMap and actualMap do not have any keys in common
func IsMapNonIntersecting(notExpectedMap map[string]string, actualMap map[string]string) bool {
	for k1 := range notExpectedMap {
		if _, ok := actualMap[k1]; ok {
			return false
		}
	}

	return true
}

type TestWriter struct {
	t *testing.T
}

func (tw TestWriter) Write(p []byte) (n int, err error) {
	tw.t.Log(string(p))
	return len(p), nil
}
