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

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"github.com/radius-project/radius/pkg/cli/helm"
	"k8s.io/apimachinery/pkg/util/wait"
	"time"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s_runtime "k8s.io/apimachinery/pkg/runtime"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/radius-project/radius/pkg/cli/output"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/kubeutil"

	// Import kubernetes auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

var (
	Scheme = k8s_runtime.NewScheme()
)

func init() {
	// Adds all types to the client.Client scheme
	// Any time we add a new type to to radius,
	// we need to add it here.
	// TODO centralize these calls.
	_ = apiextv1.AddToScheme(Scheme)
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = contourv1.AddToScheme(Scheme)
	_ = radappiov1alpha3.AddToScheme(Scheme)
}

// NewDynamicClient creates a new dynamic client by context name, otherwise returns an error.
func NewDynamicClient(context string) (dynamic.Interface, error) {
	merged, err := NewCLIClientConfig(context)
	if err != nil {
		return nil, err
	}

	client, err := dynamic.NewForConfig(merged)
	if err != nil {
		return nil, err
	}

	return client, err
}

// NewClientset creates a new Kubernetes client and rest client config by context name.
func NewClientset(context string) (*k8s.Clientset, *rest.Config, error) {
	merged, err := NewCLIClientConfig(context)
	if err != nil {
		return nil, nil, err
	}

	client, err := k8s.NewForConfig(merged)
	if err != nil {
		return nil, nil, err
	}

	return client, merged, err
}

// NewRuntimeClient creates a kubernetes client using a given context and scheme.
func NewRuntimeClient(context string, scheme *k8s_runtime.Scheme) (client.WithWatch, error) {
	merged, err := NewCLIClientConfig(context)
	if err != nil {
		return nil, err
	}

	c, err := client.NewWithWatch(merged, client.Options{Scheme: scheme})
	if err != nil {
		output.LogInfo("failed to create runtime client due to error: %v", err)
		return nil, err
	}

	return c, nil
}

// EnsureNamespace creates or get Kubernetes namespace.
//

// EnsureNamespace checks if a namespace exists in a Kubernetes cluster and creates it if it doesn't, returning an error if it fails.
func EnsureNamespace(ctx context.Context, client k8s.Interface, namespace string) error {
	namespaceApply := applycorev1.Namespace(namespace)

	// Use Apply instead of Create to avoid failures on a namespace already existing.
	_, err := client.CoreV1().Namespaces().Apply(ctx, namespaceApply, metav1.ApplyOptions{FieldManager: "rad"})
	if err != nil {
		return err
	}
	return nil
}

// deleteNamespace delete the specified namespace.
func deleteNamespace(ctx context.Context, client k8s.Interface, namespace string) error {
	if err := client.CoreV1().Namespaces().
		Delete(ctx, namespace, metav1.DeleteOptions{}); err != nil {
		return err
	}

	// Ensure the namespace is deleted. This will block until the namespace is no longer exist.
	err := wait.PollUntilContextCancel(ctx, 2*time.Second, true, func(ctx context.Context) (done bool, err error) {
		_, err = client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	return nil
}

// NewCLIClientConfig creates new Kubernetes client config loading from local home directory with CLI options.
//

// NewCLIClientConfig creates a new Kubernetes client config from the local configuration file using the given context
// name, with a default QPS and Burst. It returns a rest.Config and an error if one occurs.
func NewCLIClientConfig(context string) (*rest.Config, error) {
	return kubeutil.NewClientConfigFromLocal(&kubeutil.ConfigOptions{
		ContextName: context,
		QPS:         kubeutil.DefaultCLIQPS,
		Burst:       kubeutil.DefaultCLIBurst,
	})
}

// GetContextFromConfigFileIfExists gets context name and its context from config if context exists.
//

// GetContextFromConfigFileIfExists attempts to load a Kubernetes context from a config file, and returns an error if the
// context is not found.
func GetContextFromConfigFileIfExists(configFilePath, context string) (string, error) {
	cfg, err := kubeutil.LoadConfigFile(configFilePath)
	if err != nil {
		return "", err
	}

	contextName := context
	if contextName == "" {
		contextName = cfg.CurrentContext
	}

	if contextName == "" {
		return "", errors.New("no kubernetes context is set")
	}

	if cfg.Contexts[contextName] == nil {
		return "", fmt.Errorf("kubernetes context '%s' could not be found", contextName)
	}

	return contextName, nil
}

//go:generate mockgen -typed -destination=./mock_kubernetes.go -package=kubernetes -self_package github.com/radius-project/radius/pkg/cli/kubernetes github.com/radius-project/radius/pkg/cli/kubernetes Interface
type Interface interface {
	GetKubeContext() (*api.Config, error)
	DeleteNamespace(string) error
}

type Impl struct {
}

// Fetches the kubecontext from the system
//

// GetKubeContext loads the kube configuration file and returns a Config object and an error if the file cannot be loaded.
func (i *Impl) GetKubeContext() (*api.Config, error) {
	return kubeutil.LoadConfigFile("")
}

func (i *Impl) DeleteNamespace(kubeContext string) error {
	clientSet, _, err := NewClientset(kubeContext)
	if err != nil {
		return err
	}
	if err := deleteNamespace(context.Background(), clientSet, helm.RadiusSystemNamespace); err != nil {
		return err
	}
	return nil
}
