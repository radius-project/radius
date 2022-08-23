// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package functional

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
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
	setDefault()
	defaultDockerReg, imageTag := setDefault()
	magpieImage := "magpieimage=" + defaultDockerReg + "/magpiego:" + imageTag
	return magpieImage
}

func GetMagpieTag() string {
	setDefault()
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

func GetHostnameForHTTPProxy(ctx context.Context, client runtime_client.Client, namespace, application string) (string, error) {
	var httpproxies contourv1.HTTPProxyList

	label, err := labels.Parse(fmt.Sprintf("radius.dev/application=%s", application))
	if err != nil {
		return "", err
	}

	err = client.List(ctx, &httpproxies, &runtime_client.ListOptions{
		Namespace:     namespace,
		LabelSelector: label,
	})
	if err != nil {
		return "", err
	}

	for _, httpProxy := range httpproxies.Items {
		if httpProxy.Spec.VirtualHost != nil {
			// Found a root proxy
			return httpProxy.Spec.VirtualHost.Fqdn, nil
		}
	}

	return "", fmt.Errorf("could not find root proxy in list of cluster HTTPProxies")
}

func ExposeIngress(t *testing.T, ctx context.Context, client *k8s.Clientset, config *rest.Config, remotePort int, stopChan, readyChan chan struct{}, portChan chan int, errorChan chan error) {
	serviceName := "contour-envoy"
	label := "app.kubernetes.io/component=envoy"

	// Get the backing pod of the Ingress Service
	pods, err := client.CoreV1().Pods(RadiusSystemNamespace).List(ctx, metav1.ListOptions{LabelSelector: label, Limit: 1})
	if err != nil {
		errorChan <- err
		return
	}

	if len(pods.Items) == 0 {
		errorChan <- fmt.Errorf("no pods exist for service: %s", serviceName)
		return
	}

	pod := pods.Items[0]

	// Create API Server URL using pod name
	url := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(RadiusSystemNamespace).
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

	forwarder, err := portforward.New(dialer, []string{fmt.Sprintf(":%d", remotePort)}, stopChan, readyChan, out, errOut)
	if err != nil {
		errorChan <- err
		return
	}

	// Run the port-forward with the desired configuration
	go func() {
		errorChan <- forwarder.ForwardPorts()
	}()

	<-readyChan
	ports, err := forwarder.GetPorts()
	if err != nil {
		errorChan <- err
	}

	portChan <- int(ports[0].Local)
}

func NewTestLogger(t *testing.T) *log.Logger {
	tw := TestWriter{t}
	logger := log.Logger{}
	logger.SetOutput(tw)

	return &logger
}

type TestWriter struct {
	t *testing.T
}

func (tw TestWriter) Write(p []byte) (n int, err error) {
	tw.t.Log(string(p))
	return len(p), nil
}
