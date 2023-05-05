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
	"time"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s_runtime "k8s.io/apimachinery/pkg/runtime"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/kubeutil"
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
}

// NewDynamicClient creates a dynamic resource Kubernetes client.
//
// # Function Explanation
// 
//	NewDynamicClient creates a new dynamic client using the CLI client config from the given context, and returns it or an 
//	error if one occurs. Error handling is done by returning the error to the caller.
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

// NewClientset creates the typed Kubernetes client and return rest client config.
//
// # Function Explanation
// 
//	NewClientset creates a new Kubernetes Clientset and REST config from the given context, and returns them along with any 
//	errors encountered. If an error occurs, it should be handled by the caller.
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

// NewRuntimeClient creates a new runtime client.
//
// # Function Explanation
// 
//	NewRuntimeClient attempts to create a new Kubernetes client using the given context and scheme. It retries up to 3 times
//	 if an error occurs, and returns an error if all attempts fail. Callers should handle the error returned by this 
//	function.
func NewRuntimeClient(context string, scheme *k8s_runtime.Scheme) (client.Client, error) {
	merged, err := NewCLIClientConfig(context)
	if err != nil {
		return nil, err
	}

	var c client.Client
	for i := 0; i < 2; i++ {
		c, err = client.New(merged, client.Options{Scheme: scheme})
		if err != nil {
			output.LogInfo(fmt.Errorf("failed to get a kubernetes client: %w", err).Error())
			time.Sleep(15 * time.Second)
		}
	}
	if err != nil {
		output.LogInfo("aborting runtime client creation after 3 retries")
		return nil, err
	}

	return c, nil
}

// EnsureNamespace creates or get Kubernetes namespace.
//
// # Function Explanation
// 
//	EnsureNamespace creates a namespace in a Kubernetes cluster if it does not already exist. It handles any errors that may
//	 occur during the creation process and returns an error if one occurs.
func EnsureNamespace(ctx context.Context, client k8s.Interface, namespace string) error {
	namespaceApply := applycorev1.Namespace(namespace)

	// Use Apply instead of Create to avoid failures on a namespace already existing.
	_, err := client.CoreV1().Namespaces().Apply(ctx, namespaceApply, metav1.ApplyOptions{FieldManager: "rad"})
	if err != nil {
		return err
	}
	return nil
}

// NewCLIClientConfig creates new Kubernetes client config loading from local home directory with CLI options.
//
// # Function Explanation
// 
//	NewCLIClientConfig creates a new REST client configuration from local settings, such as the context name, QPS and Burst,
//	 and returns it or an error if one occurs.
func NewCLIClientConfig(context string) (*rest.Config, error) {
	return kubeutil.NewClientConfigFromLocal(&kubeutil.ConfigOptions{
		ContextName: context,
		QPS:         kubeutil.DefaultCLIQPS,
		Burst:       kubeutil.DefaultCLIBurst,
	})
}

// GetContextFromConfigFileIfExists gets context name and its context from config if context exists.
//
// # Function Explanation
// 
//	GetContextFromConfigFileIfExists attempts to load a config file from the given path and then checks if the given context
//	 or the current context is present in the config file. If either is found, it returns the context name, otherwise it 
//	returns an error.
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

//go:generate mockgen -destination=./mock_kubernetes.go -package=kubernetes -self_package github.com/project-radius/radius/pkg/cli/kubernetes github.com/project-radius/radius/pkg/cli/kubernetes Interface
type Interface interface {
	GetKubeContext() (*api.Config, error)
}

type Impl struct {
}

// Fetches the kubecontext from the system
//
// # Function Explanation
// 
//	The GetKubeContext function loads a configuration file and returns an api.Config object, or an error if the file cannot 
//	be loaded.
func (i *Impl) GetKubeContext() (*api.Config, error) {
	return kubeutil.LoadConfigFile("")
}
