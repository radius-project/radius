// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqv1alpha3

import (
	"context"
	"fmt"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	SecretKeyRabbitMQConnectionString = "RABBITMQ_CONNECTIONSTRING"
)

var _ renderers.Renderer = (*KubernetesRenderer)(nil)

type KubernetesRenderer struct {
}

func (r *KubernetesRenderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *KubernetesRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource := options.Resource

	properties := &radclient.RabbitMQMessageQueueResourceProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// queue name must be specified by the user
	queueName := to.String(properties.Queue)
	if queueName == "" {
		return renderers.RendererOutput{}, fmt.Errorf("queue name must be specified")
	}
	values := map[string]renderers.ComputedValueReference{
		"queue": {
			Value: queueName,
		},
	}
	if properties.Managed == nil || !*properties.Managed {
		output := renderers.RendererOutput{
			ComputedValues: values,
			SecretValues: map[string]renderers.SecretValueReference{
				"connectionString": {
					LocalID:       outputresource.LocalIDScrapedSecret,
					ValueSelector: "connectionString",
				},
			},
		}
		return output, nil
	}
	resources, err := GetRabbitMQ(resource, properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	secrets := map[string]renderers.SecretValueReference{
		"connectionString": {
			LocalID:       outputresource.LocalIDRabbitMQSecret,
			ValueSelector: SecretKeyRabbitMQConnectionString,
		},
	}

	return renderers.RendererOutput{
		Resources:      resources,
		ComputedValues: values,
		SecretValues:   secrets,
	}, nil
}

func GetRabbitMQ(resource renderers.RendererResource, properties *radclient.RabbitMQMessageQueueResourceProperties) ([]outputresource.OutputResource, error) {
	resources := []outputresource.OutputResource{}
	deployment := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
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
							Name:  "rabbitmq",
							Image: "rabbitmq:latest",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 5672,
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
		LocalID:      outputresource.LocalIDRabbitMQDeployment,
		Resource:     &deployment})

	service := corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Spec: corev1.ServiceSpec{
			Selector: kubernetes.MakeSelectorLabels(resource.ApplicationName, resource.ResourceName),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "rabbitmq",
					Port:       5672,
					TargetPort: intstr.FromInt(5672),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	uri := fmt.Sprintf("amqp://%s:%s", kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName), fmt.Sprint(5672))

	resources = append(resources, outputresource.OutputResource{
		ResourceKind: resourcekinds.Kubernetes,
		LocalID:      outputresource.LocalIDRabbitMQService,
		Resource:     &service})

	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resource.ResourceName,
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			SecretKeyRabbitMQConnectionString: []byte(uri),
		},
	}

	resources = append(resources, outputresource.OutputResource{
		ResourceKind: resourcekinds.Kubernetes,
		LocalID:      outputresource.LocalIDRabbitMQSecret,
		Resource:     &secret})

	return resources, nil
}
