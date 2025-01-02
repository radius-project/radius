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

package kubernetesclientprovider

import (
	"fmt"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/radius-project/radius/pkg/kubeutil"
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

	// Import kubernetes auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const (
	errUnsetMockText = "the Kubernetes config is nil. This is likely a test with improperly set up mocks. Use %s to set the client for testing"

	// Kind
	KindDefault ConnectionKind = "default"
	KindNone    ConnectionKind = "none"
)

// ConnectionKind is the kind of connection to use for accessing Kubernetes.
type ConnectionKind string

// Options holds the configuration options for the Kubernetes client provider.
type Options struct {
	// Kind is the kind of connection to use.
	Kind ConnectionKind `yaml:"kind"`
}

// FromOptions creates a new Kubernetes client provider from the given options.
func FromOptions(options Options) (*KubernetesClientProvider, error) {
	if options.Kind == KindNone {
		return FromConfig(nil), nil
	} else if options.Kind == KindDefault {
		config, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
			QPS:   kubeutil.DefaultServerQPS,
			Burst: kubeutil.DefaultServerBurst,
		})
		if err != nil {
			return nil, err
		}

		return FromConfig(config), nil
	}

	return nil, fmt.Errorf("unknown connection kind: %s", options.Kind)
}

// FromConfig creates a new Kubernetes client provider from the given config.
//
// For testing, pass a nil config, and then use the Set* methods to set the clients.
func FromConfig(config *rest.Config) *KubernetesClientProvider {
	return &KubernetesClientProvider{
		config: config,
	}
}

// KubernetesClientProvider provides access to Kubernetes clients.
type KubernetesClientProvider struct {
	config *rest.Config

	// These clients are all settable for testing purposes.
	clientGoClient  kubernetes.Interface
	discoveryClient discovery.DiscoveryInterface
	dynamicClient   dynamic.Interface
	runtimeClient   runtimeclient.Client
}

// Config returns the Kubernetes client provider's config.
func (k *KubernetesClientProvider) Config() *rest.Config {
	return k.config
}

// ClientGoClient returns a Kubernetes client-go client.
func (k *KubernetesClientProvider) ClientGoClient() (kubernetes.Interface, error) {
	if k.clientGoClient != nil {
		return k.clientGoClient, nil
	}

	config := k.Config()
	if config == nil {
		return nil, fmt.Errorf(errUnsetMockText, "SetClientGoClient")
	}

	return kubernetes.NewForConfig(config)
}

// SetClientGoClient sets the Kubernetes client-go client. This is useful for testing.
func (k *KubernetesClientProvider) SetClientGoClient(client kubernetes.Interface) {
	k.clientGoClient = client
}

// DiscoveryClient returns a Kubernetes discovery client.
func (k *KubernetesClientProvider) DiscoveryClient() (discovery.DiscoveryInterface, error) {
	if k.discoveryClient != nil {
		return k.discoveryClient, nil
	}

	config := k.Config()
	if config == nil {
		return nil, fmt.Errorf(errUnsetMockText, "SetDiscoveryClient")
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	// Use legacy discovery client to avoid the issue of the staled GroupVersion discovery(api.ucp.dev/v1alpha3).
	// TODO: Disable UseLegacyDiscovery once https://github.com/radius-project/radius/issues/5974 is resolved.
	client.DiscoveryClient.UseLegacyDiscovery = true
	return client, nil
}

// SetDiscoveryClient sets the Kubernetes discovery client. This is useful for testing.
func (k *KubernetesClientProvider) SetDiscoveryClient(client discovery.DiscoveryInterface) {
	k.discoveryClient = client
}

// DynamicClient returns a Kubernetes dynamic client.
func (k *KubernetesClientProvider) DynamicClient() (dynamic.Interface, error) {
	if k.dynamicClient != nil {
		return k.dynamicClient, nil
	}

	config := k.Config()
	if config == nil {
		return nil, fmt.Errorf(errUnsetMockText, "SetDiscoveryClient")
	}

	return dynamic.NewForConfig(config)
}

// SetDynamicClient sets the Kubernetes dynamic client. This is useful for testing.
func (k *KubernetesClientProvider) SetDynamicClient(client dynamic.Interface) {
	k.dynamicClient = client
}

// RuntimeClient returns a Kubernetes controller runtime client.
func (k *KubernetesClientProvider) RuntimeClient() (runtimeclient.Client, error) {
	if k.runtimeClient != nil {
		return k.runtimeClient, nil
	}

	config := k.Config()
	if config == nil {
		return nil, fmt.Errorf(errUnsetMockText, "SetRuntimeClient")
	}

	scheme := runtime.NewScheme()

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(csidriver.AddToScheme(scheme))
	utilruntime.Must(apiextv1.AddToScheme(scheme))
	utilruntime.Must(contourv1.AddToScheme(scheme))

	return runtimeclient.New(k.Config(), runtimeclient.Options{Scheme: scheme})
}

// SetRuntimeClient sets the Kubernetes controller runtime client. This is useful for testing.
func (k *KubernetesClientProvider) SetRuntimeClient(client runtimeclient.Client) {
	k.runtimeClient = client
}
