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
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v4"
	"github.com/radius-project/radius/pkg/azure/armauth"
	"github.com/radius-project/radius/pkg/azure/clientv2"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/components/trace"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/sdk"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_aws "github.com/radius-project/radius/pkg/ucp/resources/aws"
	resources_azure "github.com/radius-project/radius/pkg/ucp/resources/azure"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/otel/attribute"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceClientFactory creates a resource client using the provided Kubernetes configuration.
type ResourceClientFactory func(kubernetesConfig *rest.Config) ResourceClient

type resourceClient struct {
	arm *armauth.ArmConfig

	// armClientOptions is used to create ARM clients. Provide a Transport to override for testing.
	armClientOptions *arm.ClientOptions

	// connection is the connection to use for UCP resources. Override this for testing.
	connection sdk.Connection

	// kubernetesClient is the Kubernetes client provider used to create Kubernetes clients. Override this for testing.
	kubernetesClient *kubernetesclientprovider.KubernetesClientProvider
}

// NewResourceClient creates a new resourceClient instance with the given parameters.
func NewResourceClient(arm *armauth.ArmConfig, connection sdk.Connection, kubernetesClient *kubernetesclientprovider.KubernetesClientProvider) *resourceClient {
	return &resourceClient{arm: arm, connection: connection, kubernetesClient: kubernetesClient}
}

// NewResourceClientFactory creates resource clients that use the control-plane Kubernetes provider unless a target configuration is provided.
func NewResourceClientFactory(arm *armauth.ArmConfig, connection sdk.Connection, controlPlaneProvider *kubernetesclientprovider.KubernetesClientProvider) ResourceClientFactory {
	return func(kubernetesConfig *rest.Config) ResourceClient {
		kubernetesProvider := controlPlaneProvider
		if kubernetesConfig != nil {
			config := rest.CopyConfig(kubernetesConfig)
			config.QPS = kubeutil.DefaultServerQPS
			config.Burst = kubeutil.DefaultServerBurst
			kubernetesProvider = kubernetesclientprovider.FromConfig(config)
		}

		return NewResourceClient(arm, connection, kubernetesProvider)
	}
}

// Delete attempts to delete a resource, either through UCP, Azure, or Kubernetes, depending on the resource type.
func (c *resourceClient) Delete(ctx context.Context, id string) error {
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

	if !parsed.IsUCPQualified() || strings.HasPrefix(ns, "azure/") {
		return c.wrapError(parsed, c.deleteAzureResource(ctx, parsed))
	} else if strings.HasPrefix(ns, "kubernetes/") {
		return c.wrapError(parsed, c.deleteKubernetesResource(ctx, parsed))
	} else {
		return c.wrapError(parsed, c.deleteUCPResource(ctx, parsed))
	}
}

// GetProperties reads a UCP resource, such as an AWS resource, and returns its properties.
func (c *resourceClient) GetProperties(ctx context.Context, id string) (map[string]any, error) {
	parsed, err := resources.ParseResource(id)
	if err != nil {
		return nil, err
	}

	if !parsed.IsUCPQualified() {
		return nil, fmt.Errorf("resource %q is not a UCP resource", id)
	}

	client, err := generated.NewGenericResourcesClient(parsed.Type(), parsed.RootScope(), &aztoken.AnonymousCredential{}, sdk.NewClientOptions(c.connection))
	if err != nil {
		return nil, err
	}

	response, err := client.Get(ctx, parsed.Name(), nil)
	if err != nil {
		return nil, c.wrapError(parsed, err)
	}

	return response.Properties, nil
}

func (c *resourceClient) wrapError(id resources.ID, err error) error {
	if err != nil {
		return &ResourceError{Inner: err, ID: id.String()}
	}

	return nil
}

func (c *resourceClient) deleteAzureResource(ctx context.Context, id resources.ID) error {
	var err error
	if id.IsUCPQualified() {
		id, err = resources.ParseResource(resources.MakeRelativeID(id.ScopeSegments()[1:], id.TypeSegments(), id.ExtensionSegments()))
		if err != nil {
			return err
		}
	}

	apiVersions, err := c.lookupARMAPIVersions(ctx, id)
	if err != nil {
		return err
	}

	client, err := clientv2.NewGenericResourceClient(id.FindScope(resources_azure.ScopeSubscriptions), &c.arm.ClientOptions, c.armClientOptions)
	if err != nil {
		return err
	}

	var versionError error
	for _, apiVersion := range apiVersions {
		// Provider metadata does not map versions to regions. Ask ARM about the exact
		// resource, so child, global, and remote-region resources use their own route.
		_, err := client.GetByID(ctx, id.String(), apiVersion, nil)
		if strings.EqualFold(id.Type(), "Microsoft.Authorization/locks") {
			existence, statusCode, errorCode := "unknown", 0, ""
			if err == nil {
				existence, statusCode = "present", http.StatusOK
			} else {
				if responseError, ok := errors.AsType[*azcore.ResponseError](err); ok {
					statusCode, errorCode = responseError.StatusCode, responseError.ErrorCode
					if statusCode == http.StatusNotFound {
						switch errorCode {
						case "ResourceNotFound", "ResourceGroupNotFound", "ParentResourceNotFound":
							existence = "absent"
						}
					}
				}
			}
			// This observes existence at cleanup time, not whether this deployment created the lock.
			ucplog.FromContextOrDiscard(ctx).Info("Azure lock existence check before deletion",
				"resourceID", id.String(), "apiVersion", apiVersion,
				"existence", existence, "statusCode", statusCode, "errorCode", errorCode)
		}
		if isARMAPIVersionRejection(err) {
			versionError = err
			continue
		}
		if err != nil {
			var responseError *azcore.ResponseError
			if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
				switch responseError.ErrorCode {
				case "ResourceNotFound", "ResourceGroupNotFound", "ParentResourceNotFound", "RoleAssignmentNotFound":
					return nil
				}
			}
			return fmt.Errorf("failed to validate API version %q for resource %q: %w", apiVersion, id.String(), err)
		}

		poller, err := client.BeginDeleteByID(ctx, id.String(), apiVersion, &armresources.ClientBeginDeleteByIDOptions{})
		if isARMAPIVersionRejection(err) {
			// Only an initial ARM routing rejection permits another DELETE. Never
			// change versions after an operation has been accepted.
			versionError = err
			continue
		}
		if err != nil {
			if clients.Is404Error(err) {
				return nil
			}
			return err
		}

		_, err = poller.PollUntilDone(ctx, nil)
		if err != nil {
			if clients.Is404Error(err) {
				return nil
			}
			return err
		}

		return nil
	}

	return fmt.Errorf("no API version was accepted for resource %q: %w", id.String(), versionError)
}

func isARMAPIVersionRejection(err error) bool {
	var responseError *azcore.ResponseError
	if !errors.As(err, &responseError) || responseError.StatusCode != http.StatusBadRequest {
		return false
	}

	// These ARM errors reject the request before it reaches a resource operation.
	return responseError.ErrorCode == "NoRegisteredProviderFound" || responseError.ErrorCode == "InvalidApiVersionParameter"
}

func (c *resourceClient) lookupARMAPIVersions(ctx context.Context, id resources.ID) ([]string, error) {
	client, err := clientv2.NewProvidersClient(id.FindScope(resources_azure.ScopeSubscriptions), &c.arm.ClientOptions, c.armClientOptions)
	if err != nil {
		return nil, err
	}

	resp, err := client.Get(ctx, id.ProviderNamespace(), nil)
	if err != nil {
		return nil, err
	}

	// Provider metadata omits many nested child types, such as
	// storageAccounts/fileServices/shares. Try the exact type first, then each parent type.
	for _, shortType := range armTypeCandidates(id) {
		for _, rt := range resp.ResourceTypes {
			if rt == nil || rt.ResourceType == nil || !strings.EqualFold(shortType, *rt.ResourceType) {
				continue
			}

			versions, err := orderedAPIVersions(rt)
			if err != nil {
				return nil, fmt.Errorf("could not find API version for type %q, %w", id.Type(), err)
			}

			return versions, nil
		}
	}

	return nil, fmt.Errorf("could not find API version for type %q, type was not found", id.Type())
}

// armTypeCandidates returns provider-relative type paths for id, from the exact type to the
// top-level parent type. It uses the extension segments when the ID has them.
func armTypeCandidates(id resources.ID) []string {
	segments := id.TypeSegments()
	if len(id.ExtensionSegments()) > 0 {
		segments = id.ExtensionSegments()
	}
	if len(segments) == 0 {
		return nil
	}

	types := []string{strings.TrimPrefix(segments[0].Type, id.ProviderNamespace()+"/")}
	for _, segment := range segments[1:] {
		types = append(types, types[len(types)-1]+"/"+segment.Type)
	}
	slices.Reverse(types)
	return types
}

// orderedAPIVersions returns the unique, non-empty API versions of rt. A supported default
// version is first.
func orderedAPIVersions(rt *armresources.ProviderResourceType) ([]string, error) {
	versions := []string{}
	for _, version := range rt.APIVersions {
		if version != nil && strings.TrimSpace(*version) != "" && !slices.Contains(versions, *version) {
			versions = append(versions, *version)
		}
	}

	if len(versions) == 0 {
		return nil, errors.New("no supported API versions")
	}

	if rt.DefaultAPIVersion != nil {
		if index := slices.Index(versions, *rt.DefaultAPIVersion); index > 0 {
			version := versions[index]
			copy(versions[1:index+1], versions[:index])
			versions[0] = version
		}
	}

	return versions, nil
}

type awsOwnerKey struct{}

// WithAWSOwner returns a context whose AWS DELETEs through the resource client send owner in the
// sdkclients.AWSResourceOwnerHeader header.
func WithAWSOwner(ctx context.Context, owner string) context.Context {
	return context.WithValue(ctx, awsOwnerKey{}, owner)
}

// AWSOwnerFromContext returns the owner set with WithAWSOwner, or "".
func AWSOwnerFromContext(ctx context.Context) string {
	owner, _ := ctx.Value(awsOwnerKey{}).(string)
	return owner
}

func (c *resourceClient) deleteUCPResource(ctx context.Context, id resources.ID) error {
	// NOTE: the API version passed in here is ignored.
	//
	// We're using a generated client that understands Radius' currently supported API version.
	//
	// For AWS resources, the server does not yet validate the API version.
	//
	// In the future we should change this to look up API versions dynamically like we do for ARM.
	client, err := generated.NewGenericResourcesClient(id.Type(), id.RootScope(), &aztoken.AnonymousCredential{}, sdk.NewClientOptions(c.connection))
	if err != nil {
		return err
	}

	if owner := AWSOwnerFromContext(ctx); owner != "" && resources_aws.IsAWSResource(id) {
		ctx = policy.WithHTTPHeader(ctx, http.Header{sdkclients.AWSResourceOwnerHeader: []string{owner}})
	}

	poller, err := client.BeginDelete(ctx, id.Name(), nil)
	if err != nil {
		if clients.Is404Error(err) {
			// If the resource that we want to delete doesn't exist, we don't need to delete it.
			return nil
		}

		return err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		if clients.Is404Error(err) {
			// If the resource that we want to delete doesn't exist, we don't need to delete it.
			return nil
		}

		return err
	}

	return nil
}

func (c *resourceClient) deleteKubernetesResource(ctx context.Context, id resources.ID) error {
	apiVersion, err := c.lookupKubernetesAPIVersion(id)
	if err != nil {
		return err
	}

	group, kind, namespace, name := resources_kubernetes.ToParts(id)

	metadata := map[string]any{
		"name": name,
	}
	if namespace != "" {
		metadata["namespace"] = namespace
	}

	if group != "" {
		apiVersion = fmt.Sprintf("%s/%s", group, apiVersion)
	}

	obj := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata":   metadata,
		},
	}

	runtimeClient, err := c.kubernetesClient.RuntimeClient()
	if err != nil {
		return err
	}

	err = runtime_client.IgnoreNotFound(runtimeClient.Delete(ctx, &obj))
	if err != nil {
		return err
	}

	return nil
}

func (c *resourceClient) lookupKubernetesAPIVersion(id resources.ID) (string, error) {
	group, kind, namespace, _ := resources_kubernetes.ToParts(id)
	var resourceLists []*v1.APIResourceList

	discoveryClient, err := c.kubernetesClient.DiscoveryClient()
	if err != nil {
		return "", err
	}

	if namespace == "" {
		resourceLists, err = discoveryClient.ServerPreferredResources()
		if err != nil {
			return "", fmt.Errorf("could not find API version for type %q: %w", id.Type(), err)
		}
	} else {
		resourceLists, err = discoveryClient.ServerPreferredNamespacedResources()
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

		if group != gv.Group {
			continue
		}

		for _, resource := range resourceList.APIResources {
			if resource.Kind == kind {
				return gv.Version, nil
			}
		}
	}

	return "", fmt.Errorf("could not find API version for type %q, type was not found", id.Type())
}
