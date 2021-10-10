// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/kubernetes"
	"github.com/Azure/radius/pkg/radrp/rest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AKSDiagnosticsClient struct {
	kubernetes.KubernetesDiagnosticsClient
	ResourceClient radclient.RadiusResourceClient
	ResourceGroup  string
	SubscriptionID string
}

var _ clients.DiagnosticsClient = (*AKSDiagnosticsClient)(nil)

func (dc *AKSDiagnosticsClient) GetPublicEndpoint(ctx context.Context, options clients.EndpointOptions) (*string, error) {
	// Only HTTP Route is supported
	if len(options.ResourceID.Types) != 3 && options.ResourceID.Types[2].Type != "HttpRoute" {
		return nil, nil
	}

	response, err := dc.ResourceClient.Get(ctx, dc.ResourceGroup, options.ResourceID.Types[1].Name, options.ResourceID.Types[2].Type, options.ResourceID.Types[2].Name, nil)
	if err != nil {
		return nil, err
	}

	// Now interpret the properties to get the output resources.
	obj, ok := response.RadiusResource.Properties["status"]
	if !ok {
		return nil, nil
	}

	b, err := json.Marshal(&obj)
	if err != nil {
		return nil, nil
	}

	status := rest.ComponentStatus{}
	err = json.Unmarshal(b, &status)
	if err != nil {
		return nil, nil
	}

	for _, output := range status.OutputResources {
		gvk, ns, name, err := output.OutputResourceInfo.RequireKubernetes()
		if err != nil {
			continue // Ignore non-kubernetes
		}

		if gvk.Kind != "Ingress" {
			continue
		}

		ingress, err := dc.Client.NetworkingV1().Ingresses(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		for _, in := range ingress.Status.LoadBalancer.Ingress {
			endpoint := fmt.Sprintf("http://%s", in.IP)
			return &endpoint, nil
		}
	}

	return nil, nil
}
