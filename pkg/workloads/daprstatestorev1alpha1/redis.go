// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func GetDaprStateStoreKubernetesRedis(w workloads.InstantiatedWorkload, component DaprStateStoreComponent) ([]outputresource.OutputResource, error) {
	if !component.Config.Managed {
		return []outputresource.OutputResource{}, errors.New("only 'managed=true' is supported right now")
	}

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
				"app.kubernetes.io/name":       component.Name,
				"app.kubernetes.io/part-of":    w.Application,
				"app.kubernetes.io/managed-by": "radius-rp",
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
						"app.kubernetes.io/name":       component.Name,
						"app.kubernetes.io/part-of":    w.Application,
						"app.kubernetes.io/managed-by": "radius-rp",
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

	statestore := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "dapr.io/v1alpha1",
			"kind":       "Component",
			"metadata": map[string]interface{}{
				"name":      component.Name,
				"namespace": namespace,
				"labels": map[string]string{
					keys.LabelRadiusApplication: w.Application,
					keys.LabelRadiusComponent:   component.Name,
					// TODO get the component revision here...
					"app.kubernetes.io/name":       component.Name,
					"app.kubernetes.io/part-of":    w.Application,
					"app.kubernetes.io/managed-by": "radius-rp",
				},
			},
			"spec": map[string]interface{}{
				"type":    "state.redis",
				"version": "v1",
				"metadata": []interface{}{
					map[string]interface{}{
						"name":  "redisHost",
						"value": fmt.Sprintf("%s.%s.svc.cluster.local:6379", component.Name, namespace),
					},
					map[string]interface{}{
						"name":  "redisPassword",
						"value": "",
					},
				},
			},
		},
	}
	resources = append(resources, outputresource.OutputResource{
		Kind:     outputresource.KindKubernetes,
		LocalID:  outputresource.LocalIDDaprStateStoreComponent,
		Resource: &statestore})

	return resources, nil
}
