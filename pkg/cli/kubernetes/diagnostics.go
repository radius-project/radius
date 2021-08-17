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

	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/kubernetes"
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

	var replica *corev1.Pod

	if options.Replica != "" {
		replica, err = getSpecificReplica(ctx, dc.Client, namespace, options.Component, options.Replica)
	} else {
		replica, err = getRunningReplica(ctx, dc.Client, namespace, options.Application, options.Component)
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
		err := runPortforward(dc.RestConfig, dc.Client, replica, ready, stop, options.Port, options.RemotePort)
		failed <- err
	}()

	return
}

func (dc *KubernetesDiagnosticsClient) Logs(ctx context.Context, options clients.LogsOptions) (readCloser []clients.LogStream, err error) {
	namespace := dc.Namespace
	if namespace == "" {
		namespace = options.Application
	}

	var replicas []corev1.Pod

	if options.Replica != "" {
		replica, err := getSpecificReplica(ctx, dc.Client, namespace, options.Component, options.Replica)
		if err != nil {
			return nil, err
		}
		replicas = append(replicas, *replica)
	} else {
		replicas, err = getRunningReplicas(ctx, dc.Client, namespace, options.Application, options.Component)
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

func createLogStreams(ctx context.Context, options clients.LogsOptions, dc *KubernetesDiagnosticsClient, replicas []corev1.Pod) ([]clients.LogStream, error) {
	container := options.Container
	follow := options.Follow

	var streams []clients.LogStream
	for _, replica := range replicas {
		if container == "" {
			// We don't really expect this to fail, but let's do something reasonable if it does...
			container = getAppContainerName(&replica)
			if container == "" {
				return streams, fmt.Errorf("failed to find the default container for component '%s'. use '--container <name>' to specify the name", options.Component)
			}
		}

		stream, err := streamLogs(ctx, dc.RestConfig, dc.Client, &replica, container, follow)
		if err != nil {
			return streams, fmt.Errorf("failed to open log stream to %s: %w", options.Component, err)
		}
		streams = append(streams, clients.LogStream{Name: replica.Name, Stream: stream})
	}

	return streams, nil
}

func getSpecificReplica(ctx context.Context, client *k8s.Clientset, namespace string, component string, replica string) (*corev1.Pod, error) {
	// Right now this connects to a pod related to a component. We can find the pods with the labels
	// and then choose one that's in the running state.
	pod, err := client.CoreV1().Pods(namespace).Get(ctx, replica, v1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get replica %v for component %v: %w", replica, component, err)
	}

	if pod.Status.Phase != corev1.PodRunning {
		return nil, fmt.Errorf("replica %v for component %v is not running", replica, component)
	}

	return pod, nil
}

func getRunningReplica(ctx context.Context, client *k8s.Clientset, namespace string, application string, component string) (*corev1.Pod, error) {
	// Right now this connects to a pod related to a component. We can find the pods with the labels
	// and then choose one that's in the running state.
	pods, err := client.CoreV1().Pods(namespace).List(ctx, v1.ListOptions{
		LabelSelector: labels.FormatLabels(kubernetes.MakeSelectorLabels(application, component)),
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

func getRunningReplicas(ctx context.Context, client *k8s.Clientset, namespace string, application string, component string) ([]corev1.Pod, error) {
	// Right now this connects to a pod related to a component. We can find the pods with the labels
	// and then choose one that's in the running state.
	pods, err := client.CoreV1().Pods(namespace).List(ctx, v1.ListOptions{
		LabelSelector: labels.FormatLabels(kubernetes.MakeSelectorLabels(application, component)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list running replicas for component %v: %w", component, err)
	}
	var running []corev1.Pod
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			running = append(running, p)
		}
	}
	if len(running) == 0 {
		return nil, fmt.Errorf("failed to find a running replica for component %v", component)
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
	component := replica.Labels[kubernetes.LabelRadiusComponent]
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
