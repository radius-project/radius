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

package processors

import (
	context "context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/trace"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/otel/attribute"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// allowedDeletionRetries is the number of times to retry a UCP resource deletion request
	allowedDeletionRetries = 10

	// deletionRetryDelay is the interval to wait between UCP resource deletion retries
	deletionRetryDelay = 10 * time.Second
)

type resourceClient struct {
	arm *armauth.ArmConfig

	// armClientOptions is used to create ARM clients. Provide a Transport to override for testing.
	armClientOptions *arm.ClientOptions

	// connection is the connection to use for UCP resources. Override this for testing.
	connection sdk.Connection

	// k8sClient is the Kubernetes client used to delete Kubernetes resources. Override this for testing.
	k8sClient runtime_client.Client

	// k8sDiscoveryClient is the Kubernetes client to used for API version lookups on Kubernetes resources. Override this for testing.
	k8sDiscoveryClient discovery.ServerResourcesInterface
}

// # Function Explanation
//
// NewResourceClient creates a new resourceClient instance with the given parameters.
func NewResourceClient(arm *armauth.ArmConfig, connection sdk.Connection, k8sClient runtime_client.Client, k8sDiscoveryClient discovery.ServerResourcesInterface) *resourceClient {
	return &resourceClient{arm: arm, connection: connection, k8sClient: k8sClient, k8sDiscoveryClient: k8sDiscoveryClient}
}

// # Function Explanation
//
// Delete attempts to delete a resource, either through UCP, Azure, or Kubernetes, depending on the resource type.
func (c *resourceClient) Delete(ctx context.Context, id string, apiVersion string) error {
	parsed, err := resources.ParseResource(id)
	if err != nil {
		return err
	}

	// Performing deletion is going to fire of potentially many requests due to discovery and polling. Creating
	// a span will help categorize the requests in traces.
	attributes := []attribute.KeyValue{{Key: attribute.Key(ucplog.LogFieldTargetResourceID), Value: attribute.StringValue(id)}}
	ctx, span := trace.StartCustomSpan(ctx, "resourceclient.Delete", trace.BackendTracerName, attributes)
	defer span.End()

	// Ideally we'd do all of our resource deletion through UCP. Unfortunately we have not yet integrated
	// Azure and Kubernetes resources yet, so those are handled as special cases here.
	ns := strings.ToLower(parsed.PlaneNamespace())

	if !parsed.IsUCPQualfied() || strings.HasPrefix(ns, "azure/") {
		return c.wrapError(parsed, c.deleteAzureResource(ctx, parsed, apiVersion))
	} else if strings.HasPrefix(ns, "kubernetes/") {
		return c.wrapError(parsed, c.deleteKubernetesResource(ctx, parsed, apiVersion))
	} else {
		return c.wrapError(parsed, c.deleteUCPResource(ctx, parsed, apiVersion))
	}
}

func (c *resourceClient) wrapError(id resources.ID, err error) error {
	if err != nil {
		return &ResourceError{Inner: err, ID: id.String()}
	}

	return nil
}

func (c *resourceClient) deleteAzureResource(ctx context.Context, id resources.ID, apiVersion string) error {
	var err error
	if id.IsUCPQualfied() {
		id, err = resources.ParseResource(resources.MakeRelativeID(id.ScopeSegments()[1:], id.TypeSegments()...))
		if err != nil {
			return err
		}
	}

	if apiVersion == "" || apiVersion == resourcemodel.APIVersionUnknown {
		apiVersion, err = c.lookupARMAPIVersion(ctx, id)
		if err != nil {
			return err
		}
	}

	client, err := clientv2.NewGenericResourceClient(id.FindScope(resources.SubscriptionsSegment), &c.arm.ClientOptions, c.armClientOptions)
	if err != nil {
		return err
	}

	poller, err := client.BeginDeleteByID(ctx, id.String(), apiVersion, &armresources.ClientBeginDeleteByIDOptions{})
	if err != nil {
		return err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c *resourceClient) lookupARMAPIVersion(ctx context.Context, id resources.ID) (string, error) {
	client, err := clientv2.NewProvidersClient(id.FindScope(resources.SubscriptionsSegment), &c.arm.ClientOptions, c.armClientOptions)
	if err != nil {
		return "", err
	}

	resp, err := client.Get(ctx, id.ProviderNamespace(), nil)
	if err != nil {
		return "", err
	}

	// We need to match on the resource type name without the provider namespace.
	shortType := strings.TrimPrefix(id.TypeSegments()[0].Type, id.ProviderNamespace()+"/")
	for _, rt := range resp.ResourceTypes {
		if !strings.EqualFold(shortType, *rt.ResourceType) {
			continue
		}
		if rt.DefaultAPIVersion != nil {
			return *rt.DefaultAPIVersion, nil
		}

		if len(rt.APIVersions) > 0 {
			return *rt.APIVersions[0], nil
		}

		return "", fmt.Errorf("could not find API version for type %q, no supported API versions", id.Type())

	}

	return "", fmt.Errorf("could not find API version for type %q, type was not found", id.Type())
}

func (c *resourceClient) deleteUCPResource(ctx context.Context, id resources.ID, apiVersion string) error {
	// NOTE: the API version passed in here is ignored.
	//
	// We're using a generated client that understands Radius' currently supported API version.
	//
	// For AWS resources, the server does not yet validate the API version.
	//
	// In the future we should change this to look up API versions dynamically like we do for ARM.
	client, err := generated.NewGenericResourcesClient(id.RootScope(), id.Type(), &aztoken.AnonymousCredential{}, sdk.NewClientOptions(c.connection))
	if err != nil {
		return err
	}

	var errors []error

	// For each resource, we need to retry deletion because there may be dependent resources that are not yet deleted.
	for attempt := 1; attempt <= allowedDeletionRetries; attempt++ {
		poller, err := client.BeginDelete(ctx, id.Name(), nil)
		if err != nil {
			// If we failed to start the deletion, add the error to the list of errors and try again.
			// return err
			errors = append(errors, err)
			continue
		}

		// If we successfully started the deletion, wait for it to complete.
		r, err := poller.PollUntilDone(ctx, nil)
		if err != nil {
			fmt.Println(r)
			// If we failed to delete the resource, add the error to the list of errors and try again.
			errors = append(errors, err)
		} else {
			// If we successfully deleted the resource, we're done.
			return nil
		}

		// If we're not on the last attempt, wait a bit before trying again.
		if attempt < allowedDeletionRetries {
			time.Sleep(deletionRetryDelay)
		}
	}

	return fmt.Errorf("failed to delete resource %s after %d attempts: %v", id.Name(), allowedDeletionRetries, errors)
}

func (c *resourceClient) deleteKubernetesResource(ctx context.Context, id resources.ID, apiVersion string) error {
	if apiVersion == "" || apiVersion == resourcemodel.APIVersionUnknown {
		var err error
		apiVersion, err = c.lookupKubernetesAPIVersion(ctx, id)
		if err != nil {
			return err
		}
	}

	identity := resourcemodel.FromUCPID(id, apiVersion)
	gvk, ns, name, err := identity.RequireKubernetes()
	if err != nil {
		// We don't expect this to fail, but just in case....
		return fmt.Errorf("could not understand kubernetes resource id %q: %w", id.String(), err)
	}

	metadata := map[string]any{
		"name": name,
	}
	if ns != "" {
		metadata["namespace"] = ns
	}

	obj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": gvk.GroupVersion().String(),
			"kind":       gvk.Kind,
			"metadata":   metadata,
		},
	}

	err = runtime_client.IgnoreNotFound(c.k8sClient.Delete(ctx, &obj))
	if err != nil {
		return err
	}

	return nil
}

func (c *resourceClient) lookupKubernetesAPIVersion(ctx context.Context, id resources.ID) (string, error) {
	identity := resourcemodel.FromUCPID(id, resourcemodel.APIVersionUnknown)
	gvk, namespace, _, err := identity.RequireKubernetes()
	if err != nil {
		// We don't expect this to fail, but just in case....
		return "", fmt.Errorf("could not find API version for type %q: %w", id.Type(), err)
	}

	var resourceLists []*v1.APIResourceList
	if namespace == "" {
		resourceLists, err = c.k8sDiscoveryClient.ServerPreferredResources()
		if err != nil {
			return "", fmt.Errorf("could not find API version for type %q: %w", id.Type(), err)
		}
	} else {
		resourceLists, err = c.k8sDiscoveryClient.ServerPreferredNamespacedResources()
		if err != nil {
			return "", fmt.Errorf("could not find API version for type %q: %w", id.Type(), err)
		}
	}

	for _, resourceList := range resourceLists {
		// We know the group but not the version. This will give us the the list of resources and their preferred versions.
		gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
		if err != nil {
			return "", err
		}

		if gvk.Group != gv.Group {
			continue
		}

		for _, resource := range resourceList.APIResources {
			if resource.Kind == gvk.Kind {
				return gv.Version, nil
			}
		}
	}

	return "", fmt.Errorf("could not find API version for type %q, type was not found", id.Type())
}
