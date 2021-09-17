// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/azure/radclientv3"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/kubernetes"
	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KubernetesManagementClient struct {
	Client          client.Client
	Namespace       string
	EnvironmentName string
}

var Scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = radiusv1alpha3.AddToScheme(Scheme)
	_ = bicepv1alpha3.AddToScheme(Scheme)
}

// NOTE: for now we translate the K8s objects into the ARM format.
// see: https://github.com/Azure/radius/issues/774
var _ clients.ManagementClient = (*KubernetesManagementClient)(nil)

func (mc *KubernetesManagementClient) ListApplications(ctx context.Context) (*radclient.ApplicationList, error) {
	applications := radiusv1alpha3.ApplicationList{}
	err := mc.Client.List(ctx, &applications, &client.ListOptions{Namespace: mc.Namespace})
	if err != nil {
		return nil, err
	}

	converted := []*radclient.ApplicationResource{}
	for _, item := range applications.Items {
		application, err := ConvertK8sApplicationToARM(item)
		if err != nil {
			return nil, err
		}

		converted = append(converted, application)
	}

	if len(converted) == 0 {
		errorMessage := fmt.Sprintf("Applications not found in environment '%s'", mc.EnvironmentName)
		return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
	}

	return &radclient.ApplicationList{Value: converted}, nil
}

func (mc *KubernetesManagementClient) ShowApplication(ctx context.Context, applicationName string) (*radclient.ApplicationResource, error) {
	// We don't have a guarantee that the application name is the same
	// as the k8s resource name, so we have to filter on the client.
	applications := radiusv1alpha3.ApplicationList{}
	err := mc.Client.List(ctx, &applications, &client.ListOptions{Namespace: mc.Namespace})
	if err != nil {
		return nil, err
	}

	for _, item := range applications.Items {
		if item.Annotations[kubernetes.AnnotationsApplication] == applicationName {
			application, err := ConvertK8sApplicationToARM(item)
			if err != nil {
				return nil, err
			}

			return application, nil
		}
	}

	errorMessage := fmt.Sprintf("Application '%s' not found in environment '%s'", applicationName, mc.EnvironmentName)
	return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
}

func (mc *KubernetesManagementClient) DeleteApplication(ctx context.Context, applicationName string) error {
	// We don't have a guarantee that the application name is the same
	// as the k8s resource name, so we have to filter on the client.
	applications := radiusv1alpha3.ApplicationList{}
	err := mc.Client.List(ctx, &applications, &client.ListOptions{Namespace: mc.Namespace})
	if err != nil {
		return err
	}

	for _, item := range applications.Items {
		if item.Annotations[kubernetes.AnnotationsApplication] == applicationName {
			err = mc.Client.Delete(ctx, &item, &client.DeleteOptions{})
			if err != nil {
				return err
			}

			err = mc.deleteComponentsInApplication(ctx, applicationName)
			return err
		}
	}

	errorMessage := fmt.Sprintf("Application '%s' not found in environment '%s'", applicationName, mc.EnvironmentName)
	return radclient.NewRadiusError("ResourceNotFound", errorMessage)
}

func (mc *KubernetesManagementClient) ListComponents(ctx context.Context, applicationName string) (*radclient.ComponentList, error) {
	components := radiusv1alpha3.ResourceList{}
	err := mc.Client.List(ctx, &components, &client.ListOptions{Namespace: mc.Namespace})
	if err != nil {
		return nil, err
	}

	converted := []*radclient.ComponentResource{}
	for _, item := range components.Items {
		if item.Annotations[kubernetes.AnnotationsApplication] == applicationName {
			component, err := ConvertK8sComponentToARM(item)
			if err != nil {
				return nil, err
			}

			converted = append(converted, component)
		}
	}

	if len(converted) == 0 {
		errorMessage := fmt.Sprintf("Applications not found in environment '%s'", mc.EnvironmentName)
		return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
	}

	return &radclient.ComponentList{Value: converted}, nil
}

func (mc *KubernetesManagementClient) ShowComponent(ctx context.Context, applicationName string, componentName string) (*radclient.ComponentResource, error) {
	// We don't have a guarantee that the component name is the same
	// as the k8s resource name, so we have to filter on the client.
	components := radiusv1alpha3.ResourceList{}
	err := mc.Client.List(ctx, &components, &client.ListOptions{Namespace: mc.Namespace})
	if err != nil {
		return nil, err
	}

	for _, item := range components.Items {
		if item.Annotations[kubernetes.AnnotationsApplication] == applicationName &&
			item.Annotations[kubernetes.AnnotationsComponent] == componentName {
			component, err := ConvertK8sComponentToARM(item)
			if err != nil {
				return nil, err
			}

			return component, nil
		}
	}

	errorMessage := fmt.Sprintf("Component '%s' not found in application '%s' and environment '%s'", componentName, applicationName, mc.EnvironmentName)
	return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
}

func (mc *KubernetesManagementClient) deleteComponentsInApplication(ctx context.Context, applicationName string) error {
	components := radiusv1alpha3.ResourceList{}
	err := mc.Client.List(ctx, &components, &client.ListOptions{Namespace: mc.Namespace})
	if err != nil {
		return err
	}

	for _, item := range components.Items {
		if item.Annotations[kubernetes.AnnotationsApplication] == applicationName {
			err = mc.Client.Delete(ctx, &item, &client.DeleteOptions{})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (mc *KubernetesManagementClient) DeleteDeployment(ctx context.Context, applicationName string, deploymentName string) error {
	// We don't have a guarantee that the deployment name is the same
	// as the k8s resource name, so we have to filter on the client.
	deployments := bicepv1alpha3.DeploymentTemplateList{}
	err := mc.Client.List(ctx, &deployments, &client.ListOptions{Namespace: mc.Namespace})
	if err != nil {
		return err
	}

	for _, item := range deployments.Items {
		if item.Annotations[kubernetes.AnnotationsApplication] == applicationName &&
			item.Annotations[kubernetes.AnnotationsDeployment] == deploymentName {
			err = mc.Client.Delete(ctx, &item)
			if err != nil {
				return err
			}

			return nil
		}
	}

	errorMessage := fmt.Sprintf("Deployment '%s' not found in application '%s' environment '%s'", deploymentName, applicationName, mc.EnvironmentName)
	return radclient.NewRadiusError("ResourceNotFound", errorMessage)
}

func (mc *KubernetesManagementClient) ListDeployments(ctx context.Context, applicationName string) (*radclient.DeploymentList, error) {
	deployments := bicepv1alpha3.DeploymentTemplateList{}
	err := mc.Client.List(ctx, &deployments, &client.ListOptions{Namespace: mc.Namespace})
	if err != nil {
		return nil, err
	}

	converted := []*radclient.DeploymentResource{}
	for _, item := range deployments.Items {
		if item.Annotations[kubernetes.AnnotationsApplication] == applicationName {
			deployment, err := ConvertK8sDeploymentToARM(item)
			if err != nil {
				return nil, err
			}

			converted = append(converted, deployment)
		}
	}

	if len(converted) == 0 {
		errorMessage := fmt.Sprintf("Deployments not found in application '%s' and environment '%s'", applicationName, mc.EnvironmentName)
		return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
	}

	return &radclient.DeploymentList{Value: converted}, nil
}

func (mc *KubernetesManagementClient) ShowDeployment(ctx context.Context, applicationName string, deploymentName string) (*radclient.DeploymentResource, error) {
	// We don't have a guarantee that the deployment name is the same
	// as the k8s resource name, so we have to filter on the client.
	deployments := bicepv1alpha3.DeploymentTemplateList{}
	err := mc.Client.List(ctx, &deployments, &client.ListOptions{Namespace: mc.Namespace})
	if err != nil {
		return nil, err
	}

	for _, item := range deployments.Items {
		if item.Annotations[kubernetes.AnnotationsApplication] == applicationName &&
			item.Annotations[kubernetes.AnnotationsDeployment] == deploymentName {
			deployment, err := ConvertK8sDeploymentToARM(item)
			if err != nil {
				return nil, err
			}

			return deployment, nil
		}
	}

	errorMessage := fmt.Sprintf("Deployment '%s' not found in application '%s' environment '%s'", deploymentName, applicationName, mc.EnvironmentName)
	return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
}

func (mc *KubernetesManagementClient) ListResourcesV3(ctx context.Context, applicationName string) (*radclientv3.RadiusResourceList, error) {
	return nil, errors.New("listing V3 resources on Kubenertes is not yet supported")
}
