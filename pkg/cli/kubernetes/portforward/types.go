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
	"context"
	"io"

	k8sclient "k8s.io/client-go/kubernetes"
	rest "k8s.io/client-go/rest"
)

// Options specifies the options for port-forwarding.
type Options struct {
	// ApplicationName is the name of the application.
	ApplicationName string

	// Namespace is the kubernetes namespace of the application.
	Namespace string

	// KubeContext is the kubernetes context to use. If Client or RESTConfig is unset, this will be
	// used to initialize those fields.
	KubeContext string

	// Client is the Kubernetes client used to access the cluster. If this is set then RESTConfig
	// must also be set.
	//
	// We are using client-go here because the fake client from client-go has
	// better support for watch.
	Client k8sclient.Interface

	// Out is where output will be written.
	Out io.Writer

	// RESTConfig is the Kubernetes configuration for connecting to the server. If this is set then
	// Client must also be set.
	RESTConfig *rest.Config

	// Status chan will recieve StatusMessage updates if provided.
	StatusChan chan<- StatusMessage
}

// Note: our testing strategy for the port-forward functionality is to use a "log" of StatusMessages.
// The infrastructure will send updates via a channel that tests can listen to and block to create
// backpressure.

type StatusKind = string

const (
	KindConnected    = "connected"
	KindDisconnected = "disconnected"
)

// StatusMessage is the type used to communicate a change in port-forward status.
type StatusMessage struct {
	Kind          StatusKind
	ContainerName string
	ReplicaName   string
	LocalPort     uint16
	RemotePort    uint16
}

//go:generate mockgen -destination=./mock_portforward.go -package=portforward -self_package github.com/project-radius/radius/pkg/cli/kubernetes/portforward github.com/project-radius/radius/pkg/cli/kubernetes/portforward Interface

// Interface is the interface type for port-forwarding.
type Interface interface {
	// Run will establish port-forward connections to every Kubernetes pod that
	// is labeled as being part of the Radius application. Basing the logic on Kubernetes deployments rather
	// than Radius containers allows us to support resources created in recipes.
	//
	// Run will block until the provided context is cancelled.
	//
	// Run will allocate local ports that match the container ports of the deployments/pods where possible.
	// When a conflict occurs or when the local port is unavailable, a random port will be chosen.
	Run(ctx context.Context, options Options) error
}
