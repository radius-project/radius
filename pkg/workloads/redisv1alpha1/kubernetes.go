// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha1

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func GetKubernetesRedis(w workloads.InstantiatedWorkload, component RedisComponent) ([]outputresource.OutputResource, error) {
	// Require namespace for k8s components here.
	// Should move this check to a more generalized place.
	namespace := w.Namespace
	if namespace == "" {
		namespace = "default"
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
				keys.LabelRadiusApplication: w.Application,
				keys.LabelRadiusComponent:   component.Name,
				// TODO get the component revision here...
				keys.LabelKubernetesName:      component.Name,
				keys.LabelKubernetesPartOf:    w.Application,
				keys.LabelKubernetesManagedBy: keys.LabelKubernetesManagedByRadiusRP,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					keys.LabelRadiusApplication: w.Application,
					keys.LabelRadiusComponent:   component.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						keys.LabelRadiusApplication: w.Application,
						keys.LabelRadiusComponent:   component.Name,
						// TODO get the component revision here...
						keys.LabelKubernetesName:      component.Name,
						keys.LabelKubernetesPartOf:    w.Application,
						keys.LabelKubernetesManagedBy: keys.LabelKubernetesManagedByRadiusRP,
					},
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
				keys.LabelRadiusApplication: w.Application,
				keys.LabelRadiusComponent:   component.Name,
				// TODO get the component revision here...
				"app.kubernetes.io/name":       component.Name,
				"app.kubernetes.io/part-of":    w.Application,
				"app.kubernetes.io/managed-by": "radius-rp",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				keys.LabelRadiusApplication: w.Application,
				keys.LabelRadiusComponent:   component.Name,
			},
			Type: corev1.ServiceTypeClusterIP,
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
		Kind:     outputresource.KindKubernetes,
		LocalID:  outputresource.LocalIDRedisService,
		Resource: &service})

	return resources, nil
}

func AllocateKubernetesBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	namespace := workload.Namespace
	if namespace == "" {
		namespace = "default"
	}
	// TODO confirm workload.Name == component.Name
	host := fmt.Sprintf("%s.%s.svc.cluster.local:6379", workload.Name, namespace)
	port := fmt.Sprint(6379)
	bindings := map[string]components.BindingState{
		"redis": {
			Component: workload.Name,
			Binding:   "redis",
			Kind:      BindingKind,
			Properties: map[string]interface{}{
				"connectionString": host + ":" + port,
				"host":             host,
				"port":             port,
				"primaryKey":       "",
				"secondarykey":     "",
			},
		},
	}
	return bindings, nil
}
