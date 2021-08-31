// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package k8sauth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Azure/radius/pkg/azure/armauth"
	azclients "github.com/Azure/radius/pkg/azure/clients"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetConfig gets the Kubernetes config
func GetConfig() (*rest.Config, error) {
	var config *rest.Config
	var err error

	useLocal := os.Getenv("K8S_LOCAL")
	useCluster := os.Getenv("K8S_CLUSTER")
	if useLocal == "true" {
		config, err = createLocal()
		if err != nil {
			return nil, err
		}
	} else if useCluster == "true" {
		config, err = createCluster()
		if err != nil {
			return nil, err
		}
	} else {
		config, err = createRemote()
		if err != nil {
			return nil, err
		}
	}

	return config, nil
}

// CreateClient creates a Kubernetes client.
func CreateClient() (*client.Client, error) {
	log.Println("Creating Kubernetes Client")

	config, err := GetConfig()
	if err != nil {
		return nil, err
	}

	s := scheme.Scheme
	c, err := client.New(config, client.Options{Scheme: s})
	if err != nil {
		return nil, err
	}

	log.Println("Testing connection")
	ns := &corev1.NamespaceList{}
	err = c.List(context.Background(), ns, &client.ListOptions{})
	if err != nil {
		log.Println("Connection failed")
		return nil, fmt.Errorf("failed to connect to kubernetes ... %w", err)
	}
	log.Println("Connection verified")

	return &c, nil
}

func createLocal() (*rest.Config, error) {
	log.Println("Creating Kubernetes client based on local context")

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

	log.Println("Created Kubernetes client config")
	return config, nil
}

func createCluster() (*rest.Config, error) {
	log.Println("Creating Kubernetes client based on cluster context")

	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		return nil, err
	}

	log.Println("Created Kubernetes client config")
	return config, nil
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func createRemote() (*rest.Config, error) {
	clusterName, ok := os.LookupEnv("K8S_CLUSTER_NAME")
	if !ok {
		return nil, errors.New("required env-var K8S_CLUSTER_NAME not found")
	}

	resourceGroup, ok := os.LookupEnv("K8S_RESOURCE_GROUP")
	if !ok {
		return nil, errors.New("required env-var K8S_RESOURCE_GROUP not found")
	}

	subscriptionID, ok := os.LookupEnv("K8S_SUBSCRIPTION_ID")
	if !ok {
		return nil, errors.New("required env-var K8S_SUBSCRIPTION_ID not found")
	}

	auth, err := armauth.GetArmAuthorizer()
	if err != nil {
		return nil, fmt.Errorf("cannot authorize with ARM: %w", err)
	}

	aks := azclients.NewManagedClustersClient(subscriptionID, auth)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Println("Listing cluster credentials")
	res, err := aks.ListClusterAdminCredentials(ctx, resourceGroup, clusterName, "")
	if err != nil {
		return nil, err
	}
	log.Println("Listed cluster credentials")

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

	log.Println("Created Kubernetes client config")
	return clientconfig, nil
}
