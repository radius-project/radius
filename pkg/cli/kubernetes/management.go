// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/azure/radclientv3"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/kubernetes"
	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/radrp/schemav3"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KubernetesManagementClient struct {
	Client          client.Client
	DynamicClient   dynamic.Interface
	ExtensionClient clientset.Interface
	Namespace       string
	EnvironmentName string
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
}

func (mc *KubernetesManagementClient) ListApplicationsV3(ctx context.Context) (*radclientv3.ApplicationList, error) {
	applications := radiusv1alpha3.ApplicationList{}
	err := mc.Client.List(ctx, &applications, &client.ListOptions{Namespace: mc.Namespace})
	if err != nil {
		return nil, err
	}

	converted := []*radclientv3.ApplicationResource{}
	for _, item := range applications.Items {
		application, err := ConvertK8sApplicationToARMV3(item)
		if err != nil {
			return nil, err
		}

		converted = append(converted, application)
	}

	if len(converted) == 0 {
		errorMessage := fmt.Sprintf("Applications not found in environment '%s'", mc.EnvironmentName)
		return nil, radclientv3.NewRadiusError("ResourceNotFound", errorMessage)
	}

	return &radclientv3.ApplicationList{Value: converted}, nil
}

func (mc *KubernetesManagementClient) ShowApplicationV3(ctx context.Context, applicationName string) (*radclientv3.ApplicationResource, error) {
	application, err := mc.mustGetApplication(ctx, applicationName)
	if err != nil {
		return nil, err
	}
	return ConvertK8sApplicationToARMV3(*application)
}

func (mc *KubernetesManagementClient) DeleteApplicationV3(ctx context.Context, applicationName string) error {
	application, err := mc.mustGetApplication(ctx, applicationName)
	if err != nil {
		return err
	}

	return mc.Client.Delete(ctx, application)
}

func (mc *KubernetesManagementClient) listAllRadiusCRDs(ctx context.Context) ([]v1.CustomResourceDefinition, error) {
	crds, err := mc.ExtensionClient.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	results := []v1.CustomResourceDefinition{}
	for _, crd := range crds.Items {
		if crd.Spec.Group != radiusv1alpha3.GroupVersion.Group {
			continue
		}
		if crd.Spec.Names.Kind == schemav3.ApplicationResourceType {
			continue
		}
		for _, version := range crd.Spec.Versions {
			if version.Name != radiusv1alpha3.GroupVersion.Version {
				continue
			}
			results = append(results, crd)
		}
	}
	return results, nil
}

func (mc *KubernetesManagementClient) listAllResourcesByApplication(ctx context.Context, applicationName string, resourceType string, resourceName string) (*radclientv3.RadiusResourceList, error) {
	// First check that the application exist
	_, err := mc.mustGetApplication(ctx, applicationName)
	if err != nil {
		return nil, err
	}
	crds, err := mc.listAllRadiusCRDs(ctx)
	if err != nil {
		return nil, err
	}
	fieldSelector := map[string]string{}
	if resourceName != "" {
		fieldSelector["metadata.name"] = resourceName
	}
	labelSelector := map[string]string{kubernetes.LabelRadiusApplication: applicationName}
	if resourceType != "" {
		labelSelector[kubernetes.LabelRadiusResourceType] = resourceType
	}
	filter := metav1.ListOptions{
		FieldSelector: labels.SelectorFromSet(labels.Set(fieldSelector)).String(),
		LabelSelector: labels.SelectorFromSet(labels.Set(labelSelector)).String(),
	}
	results := []*radclientv3.RadiusResource{}
	for _, crd := range crds {
		resourceClient := mc.DynamicClient.Resource(schema.GroupVersionResource{
			Group:    radiusv1alpha3.GroupVersion.Group,
			Version:  radiusv1alpha3.GroupVersion.Version,
			Resource: crd.Spec.Names.Plural,
		}).Namespace(mc.Namespace)
		list, err := resourceClient.List(ctx, filter)
		if err != nil {
			return nil, err
		}
		for _, item := range list.Items {
			resource, err := ConvertK8sResourceToARMV3(item)
			if err != nil {
				return nil, err
			}
			results = append(results, resource)
		}
	}
	return &radclientv3.RadiusResourceList{Value: results}, nil
}

func (mc *KubernetesManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) (*radclientv3.RadiusResourceList, error) {
	return mc.listAllResourcesByApplication(ctx, applicationName, "", "")
}

func (mc *KubernetesManagementClient) ShowResource(ctx context.Context, appName string, resourceType string, resourceName string) (interface{}, error) {
	results, err := mc.listAllResourcesByApplication(ctx, appName, resourceType, resourceName)
	if err != nil {
		return nil, err
	}
	if len(results.Value) == 0 {
		errorMessage := fmt.Sprintf("Resource '%s/%s' not found in application %q and environment %q",
			resourceType, resourceName, appName, mc.EnvironmentName)
		return nil, radclientv3.NewRadiusError("ResourceNotFound", errorMessage)
	}
	return results.Value[0], nil
}

func (mc *KubernetesManagementClient) appV3NotFoundError(applicationName string) error {
	errorMessage := fmt.Sprintf("Application '%s' not found in environment '%s'", applicationName, mc.EnvironmentName)
	return radclientv3.NewRadiusError("ResourceNotFound", errorMessage)
}

// mustGetApplication will return a ResourceNotFound error if no application is found.
func (mc *KubernetesManagementClient) mustGetApplication(ctx context.Context, applicationName string) (*radiusv1alpha3.Application, error) {
	applications := radiusv1alpha3.ApplicationList{}
	err := mc.Client.List(ctx, &applications, &client.ListOptions{
		Namespace: mc.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			kubernetes.LabelRadiusApplication: applicationName,
		}),
	})
	if err != nil {
		return nil, err
	}
	if len(applications.Items) == 0 {
		return nil, mc.appV3NotFoundError(applicationName)
	}
	return &applications.Items[0], nil
}
