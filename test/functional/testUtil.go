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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
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

func ExposeIngress(ctx context.Context, client *k8s.Clientset, config *rest.Config, localHostname string, localPort int, readyChan chan struct{}, stopChan <-chan struct{}, errChan chan error) {
	namespaceName := "radius-system"
	serviceName := "contour-envoy"
	remotePort := 8080

	// Get the Ingress Service
	service, err := client.CoreV1().Services(namespaceName).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		errChan <- err
		return
	}

	labels := []string{}
	for key, val := range service.Spec.Selector {
		labels = append(labels, key+"="+val)
	}
	label := strings.Join(labels, ",")

	// Get the backing pod of the Ingress Service
	pods, err := client.CoreV1().Pods(namespaceName).List(ctx, metav1.ListOptions{LabelSelector: label, Limit: 1})
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
		Namespace(namespaceName).
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
