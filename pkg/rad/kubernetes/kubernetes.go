// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"errors"
	"os"
	"path/filepath"

	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
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

func CreateDynamicClient(context string) (dynamic.Interface, error) {
	merged, err := getConfig(context)
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
	merged, err := getConfig(context)
	if err != nil {
		return nil, nil, err
	}

	client, err := k8s.NewForConfig(merged)
	if err != nil {
		return nil, nil, err
	}

	return client, merged, err
}

func getConfig(context string) (*rest.Config, error) {
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
