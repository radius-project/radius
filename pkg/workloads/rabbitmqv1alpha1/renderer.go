// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqv1alpha1

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type Renderer struct {
}

func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	namespace := workload.Namespace
	if namespace == "" {
		namespace = workload.Application
	}

	// TODO currently we pass in an empty array of resource properties.
	// Remove once we fix component controller.
	component := RabbitMQComponent{}
	err := workload.Workload.AsRequired(Kind, &component)

	if err != nil {
		return nil, err
	}

	queueName := component.Config.Queue
	// queue name must be specified by the user
	if queueName == "" {
		return nil, fmt.Errorf("queue name must be specified")
	}

	uri := fmt.Sprintf("amqp://%s.%s.svc.cluster.local:%s", workload.Name, namespace, fmt.Sprint(5672))

	// connection string looks like amqp://NAME.NAMESPACE.svc.cluster.local:PORT
	bindings := map[string]components.BindingState{
		"rabbitmq": {
			Component: workload.Name,
			Binding:   "rabbitmq",
			Properties: map[string]interface{}{
				"connectionString": uri,
				"queue":            queueName,
			},
		},
	}
	return bindings, nil
}

// Render is the WorkloadRenderer implementation for rabbitmq workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	component := RabbitMQComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []outputresource.OutputResource{}, err
	}

	return GetRabbitMQ(w, component)
}

func GetRabbitMQ(w workloads.InstantiatedWorkload, component RabbitMQComponent) ([]outputresource.OutputResource, error) {
	// Require namespace for k8s components here.
	// Should move this check to a more generalized place.
	namespace := w.Namespace
	if namespace == "" {
		namespace = w.Application
	}

	resources := []outputresource.OutputResource{}
	deployment := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      component.Name,
			Namespace: namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(w.Application, component.Name),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: kubernetes.MakeSelectorLabels(w.Application, component.Name),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: kubernetes.MakeDescriptiveLabels(w.Application, component.Name),
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
		Kind:     outputresource.KindKubernetes,
		LocalID:  outputresource.LocalIDRabbitMQDeployment,
		Resource: &deployment})

	service := corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      component.Name,
			Namespace: namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(w.Application, component.Name),
		},
		Spec: corev1.ServiceSpec{
			Selector: kubernetes.MakeSelectorLabels(w.Application, component.Name),
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

	resources = append(resources, outputresource.OutputResource{
		Kind:     outputresource.KindKubernetes,
		LocalID:  outputresource.LocalIDRabbitMQService,
		Resource: &service})

	return resources, nil
}
