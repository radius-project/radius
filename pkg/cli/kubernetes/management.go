// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/cli/clients"
	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

type KubernetesManagementClient struct {
	Client          client.Client
	DynamicClient   dynamic.Interface
	ExtensionClient clientset.Interface
	RestClient      rest.Interface
	Namespace       string
	EnvironmentName string
	Connection      *arm.Connection
	ResourceGroup   string
	SubscriptionID  string
}

var (
	Scheme = runtime.NewScheme()

	// NOTE: for now we translate the K8s objects into the ARM format.
	// see: https://github.com/Azure/radius/issues/774
	_ clients.ManagementClient = (*KubernetesManagementClient)(nil)
)

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = radiusv1alpha3.AddToScheme(Scheme)
	_ = bicepv1alpha3.AddToScheme(Scheme)
	_ = gatewayv1alpha1.AddToScheme(Scheme)
}

func (mc *KubernetesManagementClient) ListApplications(ctx context.Context) (*radclient.ApplicationList, error) {
	ac := radclient.NewApplicationClient(mc.Connection, mc.SubscriptionID)
	response, err := ac.List(ctx, mc.ResourceGroup, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Applications not found in environment '%s'", mc.EnvironmentName)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return nil, err
	}
	return &response.ApplicationList, nil
}

func (mc *KubernetesManagementClient) ShowApplication(ctx context.Context, applicationName string) (*radclient.ApplicationResource, error) {
	ac := radclient.NewApplicationClient(mc.Connection, mc.SubscriptionID)
	response, err := ac.Get(ctx, mc.ResourceGroup, applicationName, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Application '%s' not found in environment '%s'", applicationName, mc.EnvironmentName)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return nil, err
	}
	return &response.ApplicationResource, err
}

func (mc *KubernetesManagementClient) DeleteApplication(ctx context.Context, appName string) error {
	con, sub, rg := mc.Connection, mc.SubscriptionID, mc.ResourceGroup
	radiusResourceClient := radclient.NewRadiusResourceClient(con, sub)
	resp, err := radiusResourceClient.List(ctx, mc.ResourceGroup, appName, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Application  %q not found in environment %q", appName, mc.EnvironmentName)
			return radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return err
	}
	for _, resource := range resp.RadiusResourceList.Value {
		types := strings.Split(*resource.Type, "/")
		resourceType := types[len(types)-1]
		poller, err := radclient.NewRadiusResourceClient(con, sub).BeginDelete(
			ctx, rg, appName, resourceType, *resource.Name, nil)
		if err != nil {
			return err
		}

		_, err = poller.PollUntilDone(ctx, radclient.PollInterval)
		if err != nil {
			if isNotFound(err) {
				errorMessage := fmt.Sprintf("Resource %s/%s not found in application '%s' environment '%s'",
					resourceType, *resource.Name, appName, mc.EnvironmentName)
				return radclient.NewRadiusError("ResourceNotFound", errorMessage)
			}
			return err
		}
	}
	poller, err := radclient.NewApplicationClient(con, sub).BeginDelete(ctx, rg, appName, nil)
	if err != nil {
		return err
	}
	_, err = poller.PollUntilDone(ctx, radclient.PollInterval)
	if isNotFound(err) {
		errorMessage := fmt.Sprintf("Application  %q not found in environment %q", appName, mc.EnvironmentName)
		return radclient.NewRadiusError("ResourceNotFound", errorMessage)
	}
	return err
}

func (mc *KubernetesManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) (*radclient.RadiusResourceList, error) {
	radiusResourceClient := radclient.NewRadiusResourceClient(mc.Connection, mc.SubscriptionID)

	response, err := radiusResourceClient.List(ctx, mc.ResourceGroup, applicationName, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Resources not found in application '%s' and environment '%s'", applicationName, mc.EnvironmentName)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return nil, err
	}
	return &response.RadiusResourceList, err
}

func (mc *KubernetesManagementClient) ShowResource(ctx context.Context, appName string, resourceType string, resourceName string) (interface{}, error) {
	client := radclient.NewRadiusResourceClient(mc.Connection, mc.SubscriptionID)
	result, err := client.Get(ctx, mc.ResourceGroup, appName, resourceType, resourceName, nil)
	if err != nil {
		return nil, err
	}
	return result.RadiusResource, nil
}

func isNotFound(err error) bool {
	var httpresp azcore.HTTPResponse
	ok := errors.As(err, &httpresp)
	return ok && httpresp.RawResponse().StatusCode == http.StatusNotFound
}
