// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package localrp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/cli/clients"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
)

type LocalDiagnosticsClient struct {
	K8sClient      *k8s.Clientset
	DynamicClient  dynamic.Interface
	ResourceClient radclient.RadiusResourceClient
	ResourceGroup  string
	SubscriptionID string
}

var _ clients.DiagnosticsClient = (*LocalDiagnosticsClient)(nil)

func (dc *LocalDiagnosticsClient) Expose(ctx context.Context, options clients.ExposeOptions) (failed chan error, stop chan struct{}, signals chan os.Signal, err error) {
	return nil, nil, nil, errors.New("port forwarding is not used in local environments")
}

func (dc *LocalDiagnosticsClient) Logs(ctx context.Context, options clients.LogsOptions) ([]clients.LogStream, error) {
	exe, err := dc.GetExecutable(ctx, options.Application, options.Resource)
	if err != nil {
		return nil, err
	} else if exe == nil {
		return nil, nil
	}

	for _, replica := range exe.Status.Replicas {
		log := replica.LogFile
		if log == "" {
			continue
		}

		file, err := os.Open(log)
		if err != nil {
			return nil, err
		}

		return []clients.LogStream{
			{
				Name:   options.Resource,
				Stream: file,
			},
		}, nil
	}

	return nil, nil
}

func (dc *LocalDiagnosticsClient) GetPublicEndpoint(ctx context.Context, options clients.EndpointOptions) (*string, error) {
	// Only Service is supported
	if len(options.ResourceID.Types) != 3 ||
		options.ResourceID.Types[2].Type != "Service" {
		return nil, nil
	}

	exe, err := dc.GetExecutable(ctx, options.ResourceID.Types[1].Name, options.ResourceID.Types[2].Name)
	if err != nil {
		return nil, err
	} else if exe == nil {
		return nil, nil
	}

	for _, replica := range exe.Status.Replicas {
		for _, port := range replica.Ports {
			endpoint := fmt.Sprintf("http://localhost:%d", port.Port)
			return &endpoint, nil
		}
	}

	return nil, nil
}

func (dc *LocalDiagnosticsClient) GetExecutable(ctx context.Context, applicationName string, resourceName string) (*radiusv1alpha3.Executable, error) {
	response, err := dc.ResourceClient.Get(ctx, dc.ResourceGroup, applicationName, "Service", resourceName, nil)
	if err != nil {
		return nil, err
	}

	status, err := response.RadiusResource.GetStatus()
	if err != nil {
		return nil, err
	} else if status == nil {
		return nil, nil
	}

	// TODO: Right now this is VERY coupled to how we do resource creation on the server.
	// This will be improved as part of https://github.com/Azure/radius/issues/1247 .
	//
	// When that change goes in we'll be able to work with the route type directly to get this information.
	for _, output := range status.OutputResources {
		gvk, ns, name, err := output.OutputResourceInfo.RequireKubernetes()
		if err != nil {
			continue // Ignore non-kubernetes
		}

		// If the component has a Kubernetes Executable then it should be accessible.
		if gvk.Kind != "Executable" {
			continue
		}

		executableInterface := dc.DynamicClient.Resource(schema.GroupVersionResource{
			Group:    "radius.dev",
			Version:  "v1alpha3",
			Resource: "executables",
		})
		unst, err := executableInterface.Namespace(ns).Get(ctx, name, v1.GetOptions{})
		if err != nil {
			return nil, err
		}

		b, err := unst.MarshalJSON()
		if err != nil {
			return nil, err
		}

		exe := radiusv1alpha3.Executable{}
		err = json.Unmarshal(b, &exe)
		if err != nil {
			return nil, err
		}

		return &exe, nil
	}

	return nil, nil
}
