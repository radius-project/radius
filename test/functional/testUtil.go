// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package functional

import (
	"context"
	"fmt"
	"net/http"
	"os"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func GetHostname(ctx context.Context, client runtime_client.Client) (string, error) {
	var httpproxies contourv1.HTTPProxyList

	err := client.List(ctx, &httpproxies, &runtime_client.ListOptions{})
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

func ExposeIngress(ctx context.Context, client *k8s.Clientset, config *rest.Config, localHostname string, localPort int, readyChan chan struct{}, stopChan <-chan struct{}, errChan chan error) {
	serviceName := "contour-envoy"
	label := "app.kubernetes.io/component=envoy"
	remotePort := 8080

	// Get the backing pod of the Ingress Service
	pods, err := client.CoreV1().Pods(RadiusSystemNamespace).List(ctx, metav1.ListOptions{LabelSelector: label, Limit: 1})
	if err != nil {
		errChan <- err
		return
	}

	if len(pods.Items) == 0 {
		errChan <- fmt.Errorf("no pods exist for service: %s", serviceName)
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
		errChan <- err
		return
	}

	out := os.Stdout
	errOut := os.Stderr

	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", url)

	fw, err := portforward.NewOnAddresses(dialer, []string{localHostname}, ports, stopChan, readyChan, out, errOut)
	if err != nil {
		errChan <- err
		return
	}

	// Run the port-forward with the desired configuration
	errChan <- fw.ForwardPorts()
}
