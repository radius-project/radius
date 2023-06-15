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

package portforward

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/project-radius/radius/pkg/kubernetes"
	corev1 "k8s.io/api/core/v1"
	clientgoportforward "k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type podWatcher struct {
	Cancel  func()
	Options Options
	Pod     *corev1.Pod
	Updated chan *corev1.Pod

	done      chan struct{}
	forwarder forwarder

	// forwarderOverride sets a test override for the port-forward instrastructure.
	// This allows us to test the rest of the functionality without testing actual network calls.
	forwarderOverride func(ports map[int32]bool) forwarder
	forwarderDone     chan error
	log               *bytes.Buffer
}

func NewPodWatcher(options Options, pod *corev1.Pod, cancel func()) *podWatcher {
	return &podWatcher{
		Cancel:  cancel,
		Options: options,
		Pod:     pod,

		done:          make(chan struct{}),
		forwarderDone: make(chan error),
		log:           &bytes.Buffer{},
		Updated:       make(chan *corev1.Pod),
	}
}

func (pw *podWatcher) Run(ctx context.Context) error {
	defer close(pw.done)

	// Bootstrap with initial state
	//
	// Ignore this error and keep trying if there's an issue.
	_ = pw.handleUpdate(ctx, pw.Pod)

	// Since Pods are immutable we only really need to handle one transition:
	// - The pod wasn't ready before but is ready now
	//
	// Everything else we can ignore.
	//
	// The shutdown case will be handled by cancellation of our context.
	//
	// We don't do retries for failed port-forwards.
	for {
		select {
		case <-ctx.Done():
			// Handles the case where we never started
			if pw.forwarder == nil {
				close(pw.forwarderDone)
			} else {
				<-pw.forwarderDone
			}

			return ctx.Err()

		case pod := <-pw.Updated:
			// Note: this is where we'd add retries if we wanted to.
			_ = pw.handleUpdate(ctx, pod)
		}
	}
}

func (pw *podWatcher) handleUpdate(ctx context.Context, pod *corev1.Pod) error {
	// Already listening
	if pw.forwarder != nil {
		return nil
	}

	// PodRunning is used to detect whether the pod is started or not.
	if pod == nil || pod.Status.Phase != corev1.PodRunning {
		return nil
	}

	ports := map[int32]bool{}
	for _, container := range pod.Spec.Containers {
		for _, cp := range container.Ports {
			ports[cp.ContainerPort] = true
		}
	}

	// No ports == nothing to forward
	if len(ports) == 0 {
		return nil
	}

	forwarder, err := pw.createForwarder(ports, ctx.Done(), pw.log)
	if err != nil {
		return err
	}

	pw.forwarder = forwarder

	// Forwarder will run until faulted or canceled. Use a goroutine here to unblock the eventloop.
	go pw.runForwarder(ctx)
	return nil
}

func (pw *podWatcher) runForwarder(ctx context.Context) {

	// Send notifications when ports are ready
	go func() {
		<-pw.forwarder.Ready()
		pw.sendPortNotifications(pw.forwarder, KindConnected)
	}()

	err := pw.forwarder.Run(ctx)
	pw.forwarderDone <- err

	pw.sendPortNotifications(pw.forwarder, KindDisconnected)

	close(pw.forwarderDone)
}

func (pw *podWatcher) sendPortNotifications(forwarder forwarder, kind StatusKind) {
	if pw.Options.StatusChan != nil {
		// Use Radius container name if we have one.
		containerName := pw.Pod.Labels[kubernetes.LabelRadiusResource]
		replicaName := pw.Pod.Name

		// If this is not a Radius resource then use a heuristic to get the deployment name.
		if containerName == "" {
			containerName, _, _ = strings.Cut(replicaName, "-")
		}

		ports := pw.forwarder.GetPorts()

		for _, port := range ports {
			pw.Options.StatusChan <- StatusMessage{
				Kind:          kind,
				ContainerName: containerName,
				ReplicaName:   replicaName,
				LocalPort:     port.Local,
				RemotePort:    port.Remote,
			}
		}
	}
}

func (pw *podWatcher) createForwarder(ports map[int32]bool, stopChan <-chan struct{}, output io.Writer) (forwarder, error) {
	if pw.forwarderOverride != nil {
		return pw.forwarderOverride(ports), nil
	}

	// Note: We don't really have a good way to test this code, besides E2E. This all interacts with real networks and ports
	// and requires a kubernetes config.

	transport, upgrader, err := spdy.RoundTripperFor(pw.Options.RESTConfig)
	if err != nil {
		return nil, err
	}

	url := pw.Options.Client.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(pw.Pod.Namespace).
		Name(pw.Pod.Name).
		SubResource("portforward").URL()

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", url)

	formatted := []string{}
	for remotePort := range ports {
		spec := pw.selectLocalPort(remotePort)
		formatted = append(formatted, spec)
	}

	listener, err := clientgoportforward.NewOnAddresses(dialer, []string{"localhost"}, formatted, stopChan, make(chan struct{}), output, output)
	if err != nil {
		return nil, err
	}

	return &realforwarder{inner: listener}, nil
}

func (pw *podWatcher) selectLocalPort(port int32) string {
	// We want to see if we can use the same port number for both local and remote because
	// this is simpler for users.
	//
	// First check if we can listen on the port locally.
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))

	// If there's no error we're ok!
	if err == nil {
		_ = ln.Close()
		return fmt.Sprintf("%d", port)
	}

	// If the best local port is in use then let the portforwarder pick one
	return fmt.Sprintf(":%d", port)
}

func (pw *podWatcher) Wait() {
	<-pw.done
}

type forwarder interface {
	Ready() <-chan struct{}
	Run(ctx context.Context) error
	GetPorts() []clientgoportforward.ForwardedPort
}

var _ forwarder = (*realforwarder)(nil)
var _ forwarder = (*fakeforwarder)(nil)

type realforwarder struct {
	inner *clientgoportforward.PortForwarder
}

func (f *realforwarder) Ready() <-chan struct{} {
	return f.inner.Ready
}

func (f *realforwarder) Run(ctx context.Context) error {
	return f.inner.ForwardPorts()
}

func (f *realforwarder) GetPorts() []clientgoportforward.ForwardedPort {
	ports, err := f.inner.GetPorts()
	if err != nil {
		panic("this should not happen, we only call GetPorts after the forwarder is ready")
	}
	return ports
}

func NewFakeForwarder(ports map[int32]bool) forwarder {
	fake := &fakeforwarder{ready: make(chan struct{})}
	for port := range ports {
		fake.ports = append(fake.ports, clientgoportforward.ForwardedPort{Local: uint16(port), Remote: uint16(port)})
	}

	return fake
}

type fakeforwarder struct {
	ready chan struct{}
	ports []clientgoportforward.ForwardedPort
}

func (f *fakeforwarder) Ready() <-chan struct{} {
	return f.ready
}

func (f *fakeforwarder) Run(ctx context.Context) error {
	close(f.ready)
	<-ctx.Done()
	return nil
}

func (f *fakeforwarder) GetPorts() []clientgoportforward.ForwardedPort {
	return f.ports
}
