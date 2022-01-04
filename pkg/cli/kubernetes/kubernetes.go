// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ReadKubeConfig() (*api.Config, error) {
	var kubeConfig string
	if home := homeDir(); home != "" {
		kubeConfig = filepath.Join(home, ".kube", "config")
	} else {
		return nil, errors.New("no HOME directory, cannot find kubeconfig")
	}

	config, err := clientcmd.LoadFromFile(kubeConfig)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func CreateExtensionClient(context string) (clientset.Interface, error) {
	merged, err := GetConfig(context)
	if err != nil {
		return nil, err
	}

	client, err := clientset.NewForConfig(merged)
	if err != nil {
		return nil, err
	}

	return client, err
}

func CreateRestRoundTripper(context string) (http.RoundTripper, error) {
	merged, err := GetConfig(context)
	if err != nil {
		return nil, err
	}
	gv := schema.GroupVersion{Group: "api.radius.dev", Version: "v1alpha3"}
	merged.GroupVersion = &gv
	merged.APIPath = "/apis"
	merged.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	client, err := rest.TransportFor(merged)
	if err != nil {
		return nil, err
	}

	return client, err
}

func CreateRestConfig(context string) (*rest.Config, error) {
	merged, err := GetConfig(context)
	if err != nil {
		return nil, err
	}

	return merged, err
}

func CreateDynamicClient(context string) (dynamic.Interface, error) {
	merged, err := GetConfig(context)
	if err != nil {
		return nil, err
	}

	client, err := dynamic.NewForConfig(merged)
	if err != nil {
		return nil, err
	}

	return client, err
}

func CreateTypedClient(context string) (*k8s.Clientset, *rest.Config, error) {
	merged, err := GetConfig(context)
	if err != nil {
		return nil, nil, err
	}

	client, err := k8s.NewForConfig(merged)
	if err != nil {
		return nil, nil, err
	}

	return client, merged, err
}

func CreateRuntimeClient(context string, scheme *runtime.Scheme) (client.Client, error) {
	merged, err := GetConfig(context)
	if err != nil {
		return nil, err
	}

	c, err := client.New(merged, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return c, nil
}

func EnsureNamespace(ctx context.Context, client *k8s.Clientset, namespace string) error {
	namespaceApply := applycorev1.Namespace(namespace)

	// Use Apply instead of Create to avoid failures on a namespace already existing.
	_, err := client.CoreV1().Namespaces().Apply(ctx, namespaceApply, metav1.ApplyOptions{FieldManager: "rad"})
	if err != nil {
		return err
	}
	return nil
}

func GetConfig(context string) (*rest.Config, error) {
	config, err := ReadKubeConfig()
	if err != nil {
		return nil, err
	}

	clientconfig := clientcmd.NewNonInteractiveClientConfig(*config, context, nil, nil)
	merged, err := clientconfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	return merged, err
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
