// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"bufio"
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

type ARMDiagnosticsClient struct {
	Client     *k8s.Clientset
	RestConfig *rest.Config
}

var _ clients.DiagnosticsClient = (*ARMDiagnosticsClient)(nil)

func (dc *ARMDiagnosticsClient) Expose(ctx context.Context, options clients.ExposeOptions) error {
	replica, err := getRunningReplica(ctx, dc.Client, options.Application, options.Component)
	if err != nil {
		return err
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)

	failed := make(chan error)
	ready := make(chan struct{})
	stop := make(chan struct{}, 1)
	go func() {
		err := runPortforward(dc.RestConfig, dc.Client, replica, ready, stop, options.Port, options.RemotePort)
		failed <- err
	}()

	for {
		select {
		case <-signals:
			// shutting down... wait for socket to close
			close(stop)
			continue
		case err := <-failed:
			if err != nil {
				return fmt.Errorf("failed to port-forward: %w", err)
			}

			return nil
		}
	}
}

func (dc *ARMDiagnosticsClient) Logs(ctx context.Context, options clients.LogsOptions) error {
	component := options.Component

	replica, err := getRunningReplica(ctx, dc.Client, options.Application, component)

	if err != nil {
		return err
	}

	follow := options.Follow
	container := options.Container
	if container == "" {
		// We don't really expect this to fail, but let's do something reasonable if it does...
		container = getAppContainerName(replica)
		if container == "" {
			return fmt.Errorf("failed to find the default container for component '%s'. use '--container <name>' to specify the name", component)
		}
	}

	stream, err := streamLogs(ctx, dc.RestConfig, dc.Client, replica, container, follow)
	if err != nil {
		return fmt.Errorf("failed to open log stream to %s: %w", component, err)
	}
	defer stream.Close()

	// We can keep reading this until cancellation occurs.
	if follow {
		// Sending to stderr so it doesn't interfere with parsing
		fmt.Fprintf(os.Stderr, "Streaming logs from component %s. Press CTRL+C to exit...\n", component)
	}

	hasLogs := false
	reader := bufio.NewReader(stream)
	for {
		line, prefix, err := reader.ReadLine()
		if err == context.Canceled {
			// CTRL+C => done
			return nil
		} else if err == io.EOF {
			// End of stream
			//
			// Output a status message to stderr if there were no logs for non-streaming
			// so an interactive user gets *some* feedback.
			if !follow && !hasLogs {
				fmt.Fprintln(os.Stderr, "Component's log is currently empty.")
			}

			return nil
		} else if err != nil {
			return fmt.Errorf("failed to read log stream %T: %w", err, err)
		}

		hasLogs = true

		// Handle the case where a partial line is returned
		if prefix {
			fmt.Print(string(line))
			continue
		}

		fmt.Println(string(line))
	}

	// Unreachable
}

func getRunningReplica(ctx context.Context, client *k8s.Clientset, application string, component string) (*corev1.Pod, error) {
	// Right now this connects to a pod related to a component. We can find the pods with the labels
	// and then choose one that's in the running state.
	pods, err := client.CoreV1().Pods(application).List(ctx, v1.ListOptions{
		LabelSelector: labels.FormatLabels(map[string]string{workloads.LabelRadiusComponent: component}),
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
