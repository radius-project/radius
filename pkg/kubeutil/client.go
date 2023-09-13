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

package kubeutil

import (
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	csidriver "sigs.k8s.io/secrets-store-csi-driver/apis/v1alpha1"
)

// Clients is a collection of Kubernetes clients.
type Clients struct {
	// RuntimeClient is the Kubernetes controller runtime client.
	RuntimeClient runtimeclient.Client

	// ClientSet is the Kubernetes client-go strongly-typed client.
	ClientSet *kubernetes.Clientset

	// DiscoveryClient is the Kubernetes client-go discovery client.
	DiscoveryClient *discovery.DiscoveryClient

	// DynamicClient is the Kubernetes client-go dynamic client.
	DynamicClient dynamic.Interface
}

// NewClients creates a new Kubernetes client set and controller runtime client using the given config.
func NewClients(config *rest.Config) (*Clients, error) {
	c := &Clients{}

	var err error
	c.RuntimeClient, err = NewRuntimeClient(config)
	if err != nil {
		return nil, err
	}

	c.ClientSet, err = kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	c.DiscoveryClient, err = discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}

	// Use legacy discovery client to avoid the issue of the staled GroupVersion discovery(api.ucp.dev/v1alpha3).
	// TODO: Disable UseLegacyDiscovery once https://github.com/radius-project/radius/issues/5974 is resolved.
	c.DiscoveryClient.UseLegacyDiscovery = true

	c.DynamicClient, err = dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return c, nil
}

// NewRuntimeClient creates a new runtime client using the given config and adds the
// required resource schemes to the client.
func NewRuntimeClient(config *rest.Config) (runtimeclient.Client, error) {
	scheme := runtime.NewScheme()

	// TODO: add required resource scheme.
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(csidriver.AddToScheme(scheme))
	utilruntime.Must(apiextv1.AddToScheme(scheme))
	utilruntime.Must(contourv1.AddToScheme(scheme))

	return runtimeclient.New(config, runtimeclient.Options{Scheme: scheme})
}
