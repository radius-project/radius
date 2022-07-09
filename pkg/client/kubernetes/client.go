// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/client/azuread"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type KubernetesEnvironment string

const (
	KubeLocal     KubernetesEnvironment = "local"
	KubeInCluster KubernetesEnvironment = "cluster"
	KubeAKS       KubernetesEnvironment = "azure"
)

// Options is the configuration we use for kubernetes client
type Options struct {
	// Environment represents kubernetes environment type.
	Environment KubernetesEnvironment `yaml:"environment"`
	// Azure represents azure kubernetes service option.
	Azure *AKSClusterOptions `yaml:"azure,omitempty"`
}

// AKSClusterOptions represents the options for Azure Kubernetes Service.
type AKSClusterOptions struct {
	// SubscriptionID is subscription id where AKS is created.
	SubscriptionID string `yaml:"subscriptionId"`
	// ResourceGroup is ResourceGroup where AKS is created.
	ResourceGroup string `yaml:"resourceGroup"`
	// ClusterName is AKS cluster name.
	ClusterName string `yaml:"clusterName"`

	// Identity must be set.
	Identity *azuread.Options
}

// GetKubeConfig gets the Kubernetes config
func GetKubeConfig(opts *Options) (*rest.Config, error) {
	var config *rest.Config
	var err error

	switch opts.Environment {
	case KubeLocal:
		config, err = createLocal()
		if err != nil {
			return nil, err
		}
	case KubeInCluster:
		config, err = createCluster()
		if err != nil {
			return nil, err
		}
	case KubeAKS:
		config, err = createRemote(opts.Azure)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported kubernetes environment")
	}

	return config, nil
}

func createLocal() (*rest.Config, error) {
	var kubeConfig string
	if home := homeDir(); home != "" {
		kubeConfig = filepath.Join(home, ".kube", "config")
	} else {
		return nil, errors.New("no HOME directory, cannot find kubeconfig")
	}

	log.Printf("Using %s for kube config", kubeConfig)
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func createCluster() (*rest.Config, error) {
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		return nil, err
	}
	return config, nil
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func createRemote(opts *AKSClusterOptions) (*rest.Config, error) {
	if opts == nil {
		return nil, errors.New("AKSClusterOptions is nil")
	}

	if opts.Identity == nil {
		return nil, errors.New("Identity is nil")
	}

	if opts.ClusterName == "" {
		return nil, errors.New("clusterName is unset")
	}

	if opts.ResourceGroup == "" {
		return nil, errors.New("resourceGroup is unset")
	}

	if opts.SubscriptionID == "" {
		return nil, errors.New("subscriptionId is unset")
	}

	auth, err := azuread.GetAuthorizer(opts.Identity)
	if err != nil {
		return nil, fmt.Errorf("cannot authorize with ARM: %w", err)
	}

	aks := clients.NewManagedClustersClient(opts.SubscriptionID, auth)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := aks.ListClusterAdminCredentials(ctx, opts.ResourceGroup, opts.ClusterName, "")
	if err != nil {
		return nil, err
	}

	if len(*res.Kubeconfigs) == 0 {
		return nil, errors.New("no admin credentials found")
	}

	config, err := clientcmd.NewClientConfigFromBytes(*(*res.Kubeconfigs)[0].Value)
	if err != nil {
		return nil, err
	}

	clientconfig, err := config.ClientConfig()
	if err != nil {
		return nil, err
	}

	return clientconfig, nil
}
