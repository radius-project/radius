/*
Copyright 2026 The Radius Authors.

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

package testutil

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/portforward"
)

type fakePortForwardRunner struct {
	forwardPorts func() error
	getPorts     func() ([]portforward.ForwardedPort, error)
}

func (f fakePortForwardRunner) ForwardPorts() error {
	return f.forwardPorts()
}

func (f fakePortForwardRunner) GetPorts() ([]portforward.ForwardedPort, error) {
	return f.getPorts()
}

func TestRunPortForward_ContextCanceledBeforeReadyReportsError(t *testing.T) {
	t.Parallel()

	forwardPortsStarted := make(chan struct{})
	forwardPortsRelease := make(chan struct{})
	defer close(forwardPortsRelease)

	forwarder := fakePortForwardRunner{
		forwardPorts: func() error {
			close(forwardPortsStarted)
			<-forwardPortsRelease
			return nil
		},
		getPorts: func() ([]portforward.ForwardedPort, error) {
			return nil, errors.New("GetPorts called before ready")
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	stopChan := make(chan struct{})
	readyChan := make(chan struct{})
	portChan := make(chan int)
	errorChan := make(chan error)
	done := make(chan struct{})
	go func() {
		runPortForward(ctx, forwarder, stopChan, readyChan, portChan, errorChan)
		close(done)
	}()

	<-forwardPortsStarted
	cancel()

	select {
	case err := <-errorChan:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for context cancellation error")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPortForward blocked after reporting context cancellation")
	}
}

func TestRunPortForward_ContextCanceledBeforePortReportsError(t *testing.T) {
	t.Parallel()

	forwardPortsRelease := make(chan struct{})
	defer close(forwardPortsRelease)
	getPortsStarted := make(chan struct{})
	getPortsRelease := make(chan struct{})

	forwarder := fakePortForwardRunner{
		forwardPorts: func() error {
			<-forwardPortsRelease
			return nil
		},
		getPorts: func() ([]portforward.ForwardedPort, error) {
			close(getPortsStarted)
			<-getPortsRelease
			return []portforward.ForwardedPort{{Local: 43210, Remote: 8080}}, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	stopChan := make(chan struct{})
	readyChan := make(chan struct{})
	close(readyChan)
	portChan := make(chan int)
	errorChan := make(chan error)
	done := make(chan struct{})
	go func() {
		runPortForward(ctx, forwarder, stopChan, readyChan, portChan, errorChan)
		close(done)
	}()

	<-getPortsStarted
	cancel()
	close(getPortsRelease)

	select {
	case err := <-errorChan:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for context cancellation error")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPortForward blocked after reporting context cancellation")
	}
}

func TestRunPortForward_ForwardPortsNilReportsError(t *testing.T) {
	t.Parallel()

	forwarder := fakePortForwardRunner{
		forwardPorts: func() error {
			return nil
		},
		getPorts: func() ([]portforward.ForwardedPort, error) {
			return nil, errors.New("GetPorts called before ready")
		},
	}

	stopChan := make(chan struct{})
	readyChan := make(chan struct{})
	portChan := make(chan int)
	errorChan := make(chan error)
	done := make(chan struct{})
	go func() {
		runPortForward(context.Background(), forwarder, stopChan, readyChan, portChan, errorChan)
		close(done)
	}()

	select {
	case err := <-errorChan:
		require.ErrorIs(t, err, errPortForwardStopped)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for unexpected port-forward stop error")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPortForward blocked after reporting an unexpected stop")
	}
}

func TestRunPortForward_GetPortsErrorDoesNotBlockOnForwardResult(t *testing.T) {
	t.Parallel()

	getPortsErr := errors.New("failed to get ports")
	forwardPortsErr := errors.New("forwarding stopped")
	forwardPortsRelease := make(chan struct{})
	forwardPortsReturned := make(chan struct{})
	readyChan := make(chan struct{})
	close(readyChan)

	forwarder := fakePortForwardRunner{
		forwardPorts: func() error {
			<-forwardPortsRelease
			close(forwardPortsReturned)
			return forwardPortsErr
		},
		getPorts: func() ([]portforward.ForwardedPort, error) {
			close(forwardPortsRelease)
			<-forwardPortsReturned
			return nil, getPortsErr
		},
	}

	stopChan := make(chan struct{})
	portChan := make(chan int)
	errorChan := make(chan error)
	done := make(chan struct{})
	go func() {
		runPortForward(context.Background(), forwarder, stopChan, readyChan, portChan, errorChan)
		close(done)
	}()

	select {
	case err := <-errorChan:
		require.ErrorIs(t, err, getPortsErr)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for GetPorts error")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPortForward blocked after reporting the GetPorts error")
	}

	select {
	case err := <-errorChan:
		t.Fatalf("received unexpected second error: %v", err)
	default:
	}
}

func TestRunPortForward_ForwardsRuntimeError(t *testing.T) {
	t.Parallel()

	forwardPortsErr := errors.New("forwarding stopped")
	forwardPortsRelease := make(chan struct{})
	readyChan := make(chan struct{})
	close(readyChan)

	forwarder := fakePortForwardRunner{
		forwardPorts: func() error {
			<-forwardPortsRelease
			return forwardPortsErr
		},
		getPorts: func() ([]portforward.ForwardedPort, error) {
			return []portforward.ForwardedPort{{Local: 43210, Remote: 8080}}, nil
		},
	}

	stopChan := make(chan struct{})
	portChan := make(chan int)
	errorChan := make(chan error)
	go runPortForward(context.Background(), forwarder, stopChan, readyChan, portChan, errorChan)

	select {
	case port := <-portChan:
		require.Equal(t, 43210, port)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for forwarded port")
	}

	close(forwardPortsRelease)
	select {
	case err := <-errorChan:
		require.ErrorIs(t, err, forwardPortsErr)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runtime forwarding error")
	}
}
