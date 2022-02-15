// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converters

import (
	"github.com/project-radius/radius/pkg/kubernetes"
	radruntime "github.com/project-radius/radius/pkg/kubernetes/api/radius/runtime/v1alpha3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ContainerToDeployment(container *radruntime.Container) *appsv1.Deployment {
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      container.Name,
			Namespace: container.Namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(container.Spec.ApplicationName, container.Spec.ResourceName),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: kubernetes.MakeSelectorLabels(container.Spec.ApplicationName, container.Spec.ResourceName),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: kubernetes.MakeDescriptiveLabels(container.Spec.ApplicationName, container.Spec.ResourceName),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						*container.Spec.Container.DeepCopy(),
					},
					Volumes: container.Spec.Volumes,
				},
			},
		},
	}
	return &deployment
}
