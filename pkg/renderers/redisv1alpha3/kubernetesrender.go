// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha3

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ renderers.Renderer = (*KubernetesRenderer)(nil)

type KubernetesRenderer struct {
}

func (r *KubernetesRenderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r *KubernetesRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource := options.Resource

	properties := RedisComponentProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	if !properties.Managed {
		return renderers.RendererOutput{}, fmt.Errorf("only managed = true is supported for the Kubernetes Redis Component")
	}

	resources, err := GetKubernetesRedis(resource, properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// For now we don't know the namespace during rendering and so we can't generate a FQDN, so use a simple
	// one. This should be fine because all of the application's pods are in the same namespace as this
	// service.
	host := kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName)
	port := fmt.Sprint(Port)
	output := renderers.RendererOutput{
		Resources: resources,
		ComputedValues: map[string]renderers.ComputedValueReference{
			"host": {
				Value: host,
			},
			"port": {
				Value: port,
			},
			"username": {
				Value: "",
			},
			// NOTE: these are not secrets because they are blank. If we start generating
			// secret credentials here, then this will need to change to use secrets.
			"password": {
				Value: "",
			},
		},
	}
	return output, nil
}

func GetKubernetesRedis(resource renderers.RendererResource, properties RedisComponentProperties) ([]outputresource.OutputResource, error) {
	resources := []outputresource.OutputResource{}
	deployment := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
			Labels: kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: kubernetes.MakeSelectorLabels(resource.ApplicationName, resource.ResourceName),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: "redis",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 6379,
									Protocol:      corev1.ProtocolTCP,
								},
							},
						},
					},
				},
			},
		},
	}

	resources = append(resources, outputresource.OutputResource{
		ResourceKind: resourcekinds.Kubernetes,
		LocalID:      outputresource.LocalIDRedisDeployment,
		Resource:     &deployment})

	service := corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
			Labels: kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Spec: corev1.ServiceSpec{
			Selector: kubernetes.MakeSelectorLabels(resource.ApplicationName, resource.ResourceName),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	resources = append(resources, outputresource.OutputResource{
		ResourceKind: resourcekinds.Kubernetes,
		LocalID:      outputresource.LocalIDRedisService,
		Resource:     &service})

	return resources, nil
}
