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

	properties := resources[0].Properties
	// queue name must be specified by the user
	queueName, ok := properties[QueueNameKey]
	if !ok {
		return nil, fmt.Errorf("missing required property '%s'", QueueNameKey)
	}

	host := fmt.Sprintf("amqp://%s.%s.svc.cluster.local", workload.Name, namespace)
	port := fmt.Sprint(6379)

	// connection string looks like amqp://NAME.NAMESPACE.svc.cluster.local:PORT
	bindings := map[string]components.BindingState{
		"rabbitmq": {
			Component: workload.Name,
			Binding:   "rabbitmq",
			Properties: map[string]interface{}{
				"connectionString": host + ":" + port,
				"queue":            queueName,
			},
		},
	}
	return bindings, nil
}

// Render is the WorkloadRenderer implementation for redis workload.
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
			Labels: map[string]string{
				kubernetes.LabelRadiusApplication: w.Application,
				kubernetes.LabelRadiusComponent:   component.Name,
				// TODO get the component revision here...
				kubernetes.LabelName:      component.Name,
				kubernetes.LabelPartOf:    w.Application,
				kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					kubernetes.LabelRadiusApplication: w.Application,
					kubernetes.LabelRadiusComponent:   component.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kubernetes.LabelRadiusApplication: w.Application,
						kubernetes.LabelRadiusComponent:   component.Name,
						// TODO get the component revision here...
						kubernetes.LabelName:      component.Name,
						kubernetes.LabelPartOf:    w.Application,
						kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "rabbitmq",
							Image: "rabbitmq:3-management", // TODO confirm which image to use
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 5672,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									ContainerPort: 15672,
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
		LocalID:  outputresource.LocalIDRedisDeployment,
		Resource: &deployment})

	service := corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      component.Name,
			Namespace: namespace,
			Labels: map[string]string{
				kubernetes.LabelRadiusApplication: w.Application,
				kubernetes.LabelRadiusComponent:   component.Name,
				// TODO get the component revision here...
				kubernetes.LabelName:      component.Name,
				kubernetes.LabelPartOf:    w.Application,
				kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				kubernetes.LabelRadiusApplication: w.Application,
				kubernetes.LabelRadiusComponent:   component.Name,
			},
			Type: corev1.ServiceTypeClusterIP,
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
		LocalID:  outputresource.LocalIDRedisService,
		Resource: &service})

	return resources, nil
}
