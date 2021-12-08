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
	"strings"

	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/kubernetes"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KubernetesDiagnosticsClient struct {
	K8sClient  *k8s.Clientset
	RestConfig *rest.Config
	Client     client.Client

	// If set, the value of this field will be used to find replicas. Otherwise the application name will be used.
	Namespace string
}

var _ clients.DiagnosticsClient = (*KubernetesDiagnosticsClient)(nil)

func (dc *KubernetesDiagnosticsClient) GetPublicEndpoint(ctx context.Context, options clients.EndpointOptions) (*string, error) {
	if len(options.ResourceID.Types) != 3 || !strings.EqualFold(options.ResourceID.Types[2].Type, resourcekinds.RadiusHttpRoute) {
		return nil, nil
	}

	httproute := radiusv1alpha3.HttpRoute{}

	name := options.ResourceID.Types[1].Name + "-" + options.ResourceID.Types[2].Name
	err := dc.Client.Get(ctx, types.NamespacedName{Namespace: options.ResourceID.ResourceGroup, Name: name}, &httproute)
	if err != nil {
		return nil, err
	}

	// TODO: Right now this is VERY coupled to how we do resource creation on the server.
	// This will be improved as part of https://github.com/Azure/radius/issues/1247 .
	//
	// When that change goes in we'll be able to work with the route type directly to get this information.
	for _, output := range httproute.Status.Resources {
		// If the component has a Kubernetes HTTPRoute then it's using gateways. Look up the IP address
		if output.Resource.Kind != resourcekinds.KubernetesHTTPRoute {
			continue
		}

		service, err := dc.K8sClient.CoreV1().Services("radius-system").Get(ctx, "haproxy-ingress", metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		for _, in := range service.Status.LoadBalancer.Ingress {
			endpoint := fmt.Sprintf("http://%s", in.IP)
			return &endpoint, nil
		}
	}

	return nil, nil
}

func (dc *KubernetesDiagnosticsClient) Expose(ctx context.Context, options clients.ExposeOptions) (failed chan error, stop chan struct{}, signals chan os.Signal, err error) {
	namespace := dc.Namespace
	if namespace == "" {
		namespace = options.Application
	}

	var replica *corev1.Pod

	if options.Replica != "" {
		replica, err = getSpecificReplica(ctx, dc.K8sClient, namespace, options.Resource, options.Replica)
	} else {
		replica, err = getRunningReplica(ctx, dc.K8sClient, namespace, options.Application, options.Resource)
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
		err := runPortforward(dc.RestConfig, dc.K8sClient, replica, ready, stop, options.Port, options.RemotePort)
		failed <- err
	}()

	return
}

func (dc *KubernetesDiagnosticsClient) Logs(ctx context.Context, options clients.LogsOptions) ([]clients.LogStream, error) {
	namespace := dc.Namespace
	if namespace == "" {
		namespace = options.Application
	}

	var replicas []corev1.Pod
	var err error

	if options.Replica != "" {
		replica, err := getSpecificReplica(ctx, dc.K8sClient, namespace, options.Resource, options.Replica)
		if err != nil {
			return nil, err
		}
		replicas = append(replicas, *replica)
	} else {
		replicas, err = getRunningReplicas(ctx, dc.K8sClient, namespace, options.Application, options.Resource)
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

// Note: If an error is returned, any streams that were created before the error will also be returned.
// Caller is responsible for closing streams even when there is an error.
func createLogStreams(ctx context.Context, options clients.LogsOptions, dc *KubernetesDiagnosticsClient, replicas []corev1.Pod) ([]clients.LogStream, error) {
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

		stream, err := streamLogs(ctx, dc.RestConfig, dc.K8sClient, &replica, container, follow)
		if err != nil {
			return streams, fmt.Errorf("failed to open log stream to %s: %w", options.Resource, err)
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
	// The container name will be the resource name
	resource := replica.Labels[kubernetes.LabelRadiusResource]
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
