// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
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
	if len(options.ResourceID.Types) != 3 || !strings.EqualFold(options.ResourceID.Types[2].Type, resourcekinds.RadiusHttpRoute) {
		return nil, nil
	}

	response, err := dc.ResourceClient.Get(ctx, dc.ResourceGroup, options.ResourceID.Types[1].Name, options.ResourceID.Types[2].Type, options.ResourceID.Types[2].Name, nil)
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
	// This will be improved as part of https://github.com/project-radius/radius/issues/1247 .
	//
	// When that change goes in we'll be able to work with the route type directly to get this information.
	for _, output := range status.OutputResources {
		gvk, _, _, err := output.OutputResourceInfo.RequireKubernetes()
		if err != nil {
			continue // Ignore non-kubernetes
		}

		// If the container has a Kubernetes HTTPRoute then it's using gateways. Look up the IP address
		if gvk.Kind != resourcekinds.KubernetesHTTPRoute {
			continue
		}

		service, err := dc.K8sClient.CoreV1().Services("radius-system").Get(ctx, "haproxy-ingress", metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		for _, in := range service.Status.LoadBalancer.Ingress {
			endpoint := fmt.Sprintf("http://%s", in.IP)
			return &endpoint, nil
		}
	}

	return nil, nil
}
