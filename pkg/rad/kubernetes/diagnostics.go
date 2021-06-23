// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"

	"github.com/Azure/radius/pkg/rad/clients"
	"github.com/Azure/radius/pkg/workloads"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type KubernetesDiagnosticsClient struct {
	Client     *k8s.Clientset
	RestConfig *rest.Config

	// If set, the value of this field will be used to find replicas. Otherwise the application name will be used.
	Namespace string
}

var _ clients.DiagnosticsClient = (*KubernetesDiagnosticsClient)(nil)

func (dc *KubernetesDiagnosticsClient) Expose(ctx context.Context, options clients.ExposeOptions) (failed chan error, stop chan struct{}, signals chan os.Signal, err error) {
	namespace := dc.Namespace
	if namespace == "" {
		namespace = options.Application
	}

	replica, err := getRunningReplica(ctx, dc.Client, namespace, options.Application, options.Component)
	if err != nil {
		return
	}

	signals = make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	failed = make(chan error)
	ready := make(chan struct{})
	stop = make(chan struct{}, 1)
	go func() {
		err := runPortforward(dc.RestConfig, dc.Client, replica, ready, stop, options.Port, options.RemotePort)
		failed <- err
	}()

	return
}

func (dc *KubernetesDiagnosticsClient) Logs(ctx context.Context, options clients.LogsOptions) (io.ReadCloser, error) {
	namespace := dc.Namespace
	if namespace == "" {
		namespace = options.Application
	}

	replica, err := getRunningReplica(ctx, dc.Client, namespace, options.Application, options.Component)

	if err != nil {
		return nil, err
	}

	follow := options.Follow
	container := options.Container
	if container == "" {
		// We don't really expect this to fail, but let's do something reasonable if it does...
		container = getAppContainerName(replica)
		if container == "" {
			return nil, fmt.Errorf("failed to find the default container for component '%s'. use '--container <name>' to specify the name", options.Component)
		}
	}

	stream, err := streamLogs(ctx, dc.RestConfig, dc.Client, replica, container, follow)
	if err != nil {
		return nil, fmt.Errorf("failed to open log stream to %s: %w", options.Component, err)
	}

	return stream, err
}

func getRunningReplica(ctx context.Context, client *k8s.Clientset, namespace string, application string, component string) (*corev1.Pod, error) {
	// Right now this connects to a pod related to a component. We can find the pods with the labels
	// and then choose one that's in the running state.
	pods, err := client.CoreV1().Pods(namespace).List(ctx, v1.ListOptions{
		LabelSelector: labels.FormatLabels(map[string]string{
			workloads.LabelRadiusApplication: application,
			workloads.LabelRadiusComponent:   component,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list running replicas for component %v: %w", component, err)
	}

	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			return &p, nil
		}
	}

	return nil, fmt.Errorf("failed to find a running replica for component %v", component)
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

	out := ioutil.Discard
	errOut := ioutil.Discard
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
	// The container name will be the component name
	component := replica.Labels[workloads.LabelRadiusComponent]
	return component
}

func streamLogs(ctx context.Context, config *rest.Config, client *k8s.Clientset, replica *corev1.Pod, container string, follow bool) (io.ReadCloser, error) {
	options := &corev1.PodLogOptions{
		Container: container,
		Follow:    follow,
	}

	request := client.CoreV1().Pods(replica.Namespace).GetLogs(replica.Name, options)
	return request.Stream(ctx)
}
