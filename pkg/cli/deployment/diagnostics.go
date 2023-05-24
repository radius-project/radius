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

package deployment

import (
	"context"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	k8slabels "github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/ucp/resources"

	"io"
	"net/http"
	"os"
	"os/signal"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ARMDiagnosticsClient struct {
	K8sTypedClient    *k8s.Clientset
	RestConfig        *rest.Config
	K8sRuntimeClient  client.Client
	ApplicationClient generated.GenericResourcesClient
	ContainerClient   generated.GenericResourcesClient
	EnvironmentClient generated.GenericResourcesClient
	GatewayClient     generated.GenericResourcesClient
}

var _ clients.DiagnosticsClient = (*ARMDiagnosticsClient)(nil)

func (dc *ARMDiagnosticsClient) GetPublicEndpoint(ctx context.Context, options clients.EndpointOptions) (*string, error) {
	if !strings.EqualFold("Applications.Core/gateways", options.ResourceID.Type()) {
		return nil, nil
	}

	response, err := dc.GatewayClient.Get(ctx, options.ResourceID.Name(), nil)
	if err != nil {
		return nil, err
	}

	obj, ok := response.Properties["url"]
	if !ok {
		return nil, fmt.Errorf("could not find URL for gateway %q", options.ResourceID.Name())
	}

	url, ok := obj.(string)
	if !ok {
		return nil, fmt.Errorf("could not find URL for gateway %q", options.ResourceID.Name())
	}

	return &url, nil
}

func (dc *ARMDiagnosticsClient) Expose(ctx context.Context, options clients.ExposeOptions) (failed chan error, stop chan struct{}, signals chan os.Signal, err error) {
	namespace, err := dc.findNamespaceOfContainer(ctx, options.Resource)
	if err != nil {
		return
	}

	var replica *corev1.Pod
	if options.Replica != "" {
		replica, err = getSpecificReplica(ctx, dc.K8sTypedClient, namespace, options.Resource, options.Replica)
	} else {
		replica, err = getRunningReplica(ctx, dc.K8sTypedClient, namespace, options.Application, options.Resource)
	}

	if err != nil {
		return
	}

	fmt.Printf("Exposing replica %s\n", replica.Name)

	signals = make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	failed = make(chan error)
	ready := make(chan struct{})
	stop = make(chan struct{}, 1)
	go func() {
		err := runPortforward(dc.RestConfig, dc.K8sTypedClient, replica, ready, stop, options.Port, options.RemotePort)
		failed <- err
	}()

	return
}

func (dc *ARMDiagnosticsClient) Logs(ctx context.Context, options clients.LogsOptions) ([]clients.LogStream, error) {
	namespace, err := dc.findNamespaceOfContainer(ctx, options.Resource)
	if err != nil {
		return nil, nil
	}

	var replicas []corev1.Pod

	if options.Replica != "" {
		replica, err := getSpecificReplica(ctx, dc.K8sTypedClient, namespace, options.Resource, options.Replica)
		if err != nil {
			return nil, err
		}
		replicas = append(replicas, *replica)
	} else {
		replicas, err = getRunningReplicas(ctx, dc.K8sTypedClient, namespace, options.Application, options.Resource)
		if err != nil {
			return nil, err
		}
	}

	streams, err := createLogStreams(ctx, options, dc, replicas)
	if err != nil {
		// If there was an error, try to close all streams that were created
		// ignore errors from stream close
		for _, stream := range streams {
			_ = stream.Stream.Close()
		}
		return nil, err
	}

	return streams, err
}

func (dc *ARMDiagnosticsClient) findNamespaceOfContainer(ctx context.Context, resourceName string) (string, error) {
	containerResponse, err := dc.ContainerClient.Get(ctx, resourceName, nil)
	if err != nil {
		return "", fmt.Errorf("could not find container %q:%w", resourceName, err)
	}

	obj, ok := containerResponse.Properties["application"]
	if !ok {
		return "", fmt.Errorf("could not find namespace for container %q", resourceName)
	}

	application, ok := obj.(string)
	if !ok {
		return "", fmt.Errorf("could not find namespace for container %q", resourceName)
	}

	id, err := resources.ParseResource(application)
	if err != nil {
		return "", fmt.Errorf("could not namespace for container %q:%w", resourceName, err)
	}

	applicationResponse, err := dc.ApplicationClient.Get(ctx, id.Name(), nil)
	if err != nil {
		return "", fmt.Errorf("could not namespace for container %q:%w", resourceName, err)
	}

	obj, ok = applicationResponse.Properties["status"]
	if !ok {
		return "", fmt.Errorf("could not find namespace for container %q", resourceName)
	}

	status, ok := obj.(map[string]any)
	if !ok {
		return "", fmt.Errorf("could not find namespace for container %q", resourceName)
	}

	obj, ok = status["compute"]
	if !ok {
		return "", fmt.Errorf("could not find namespace for container %q", resourceName)
	}

	compute, ok := obj.(map[string]any)
	if !ok {
		return "", fmt.Errorf("could not find namespace for container %q", resourceName)
	}

	kind, ok := compute["kind"].(string)
	if !ok || !strings.EqualFold(kind, "kubernetes") {
		return "", fmt.Errorf("could not find namespace for container %q", resourceName)
	}

	namespace, ok := compute["namespace"].(string)
	if ok {
		return namespace, nil
	}

	return "", fmt.Errorf("could not find namespace for container %q", resourceName)
}

// Note: If an error is returned, any streams that were created before the error will also be returned.
// Caller is responsible for closing streams even when there is an error.
func createLogStreams(ctx context.Context, options clients.LogsOptions, dc *ARMDiagnosticsClient, replicas []corev1.Pod) ([]clients.LogStream, error) {
	container := options.Container
	follow := options.Follow

	var streams []clients.LogStream
	for _, replica := range replicas {
		if container == "" {
			// We don't really expect this to fail, but let's do something reasonable if it does...
			container = getAppContainerName(&replica)
			if container == "" {
				return streams, fmt.Errorf("failed to find the default container for resource '%s'. use '--container <name>' to specify the name", options.Resource)
			}
		}

		stream, err := streamLogs(ctx, dc.RestConfig, dc.K8sTypedClient, &replica, container, follow)
		if err != nil {
			return streams, fmt.Errorf("failed to open log stream to %s: %w", options.Resource, err)
		}
		streams = append(streams, clients.LogStream{Name: replica.Name, Stream: stream})
	}

	return streams, nil
}

func getSpecificReplica(ctx context.Context, client *k8s.Clientset, namespace string, resource string, replica string) (*corev1.Pod, error) {
	// Right now this connects to a pod related to a resource. We can find the pods with the labels
	// and then choose one that's in the running state.
	pod, err := client.CoreV1().Pods(namespace).Get(ctx, replica, v1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get replica %v for resource %v: %w", replica, resource, err)
	}

	if pod.Status.Phase != corev1.PodRunning {
		return nil, fmt.Errorf("replica %v for resource %v is not running", replica, resource)
	}

	return pod, nil
}

func getRunningReplica(ctx context.Context, client *k8s.Clientset, namespace string, application string, resource string) (*corev1.Pod, error) {
	// Right now this connects to a pod related to a resource. We can find the pods with the labels
	// and then choose one that's in the running state.
	pods, err := client.CoreV1().Pods(namespace).List(ctx, v1.ListOptions{
		LabelSelector: labels.FormatLabels(k8slabels.MakeSelectorLabels(application, resource)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list running replicas for resource %v: %w", resource, err)
	}

	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			return &p, nil
		}
	}

	return nil, fmt.Errorf("failed to find a running replica for resource %v", resource)
}

func getRunningReplicas(ctx context.Context, client *k8s.Clientset, namespace string, application string, resource string) ([]corev1.Pod, error) {
	// Right now this connects to a pod related to a resource. We can find the pods with the labels
	// and then choose one that's in the running state.
	pods, err := client.CoreV1().Pods(namespace).List(ctx, v1.ListOptions{
		LabelSelector: labels.FormatLabels(k8slabels.MakeSelectorLabels(application, resource)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list running replicas for resource %v: %w", resource, err)
	}
	var running []corev1.Pod
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			running = append(running, p)
		}
	}
	if len(running) == 0 {
		return nil, fmt.Errorf("failed to find a running replica for resource %v", resource)
	}

	return running, nil
}

func runPortforward(restconfig *rest.Config, client *k8s.Clientset, replica *corev1.Pod, ready chan struct{}, stop <-chan struct{}, localPort int, remotePort int) error {
	// Build URL so we can open a port-forward via SPDY
	url := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(replica.Namespace).
		Name(replica.Name).
		SubResource("portforward").URL()

	transport, upgrader, err := spdy.RoundTripperFor(restconfig)
	if err != nil {
		return err
	}

	out := io.Discard
	errOut := io.Discard
	if true {
		out = os.Stdout
		errOut = os.Stderr
	}

	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", url)

	fw, err := portforward.NewOnAddresses(dialer, []string{"localhost"}, ports, stop, ready, out, errOut)
	if err != nil {
		return err
	}

	return fw.ForwardPorts()
}

func getAppContainerName(replica *corev1.Pod) string {
	// The container name will be the resource name
	resource := replica.Labels[k8slabels.LabelRadiusResource]
	return resource
}

func streamLogs(ctx context.Context, config *rest.Config, client *k8s.Clientset, replica *corev1.Pod, container string, follow bool) (io.ReadCloser, error) {
	options := &corev1.PodLogOptions{
		Container: container,
		Follow:    follow,
	}

	request := client.CoreV1().Pods(replica.Namespace).GetLogs(replica.Name, options)
	return request.Stream(ctx)
}
