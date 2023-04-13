// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
func NewCLIClientConfig(context string) (*rest.Config, error) {
	return kubeutil.NewClientConfigFromLocal(&kubeutil.ConfigOptions{
		ContextName: context,
		QPS:         kubeutil.DefaultCLIQPS,
		Burst:       kubeutil.DefaultCLIBurst,
	})
}

// GetContextFromConfigFileIfExists gets context name and its context from config if context exists.
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
func (i *Impl) GetKubeContext() (*api.Config, error) {
	return kubeutil.LoadConfigFile("")
}
