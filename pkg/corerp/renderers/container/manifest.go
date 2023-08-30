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

package container

import (
	"strings"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/kubeutil"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func fetchBaseManifest(r *datamodel.ContainerResource) (kubeutil.ObjectManifest, error) {
	baseManifest := kubeutil.ObjectManifest{}
	runtimes := r.Properties.Runtimes
	var err error

	if runtimes != nil && runtimes.Kubernetes != nil && runtimes.Kubernetes.Base != "" {
		baseManifest, err = kubeutil.ParseManifest([]byte(runtimes.Kubernetes.Base))
		if err != nil {
			return nil, err
		}
	}

	return baseManifest, nil
}

func getServiceBase(manifest kubeutil.ObjectManifest, appName string, r *datamodel.ContainerResource, options *renderers.RenderOptions) *corev1.Service {
	// If the service has a base manifest, get the service resource from the base manifest.
	// Otherwise, populate default resources.
	defaultService := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{},
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
	if resource := manifest.GetFirst(kubeutil.ServiceV1); resource != nil {
		defaultService = resource.(*corev1.Service)
	}
	defaultService.ObjectMeta = getObjectMeta(defaultService.ObjectMeta, appName, r.Name, r.ResourceTypeName(), *options)
	return defaultService
}

func getDeploymentBase(manifest kubeutil.ObjectManifest, appName string, r *datamodel.ContainerResource, options *renderers.RenderOptions) *appsv1.Deployment {
	name := kubernetes.NormalizeResourceName(r.Name)

	// If the container has a base manifest, get the deployment resource from the base manifest.
	// Otherwise, populate default resources.
	defaultDeployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{},
					Annotations: map[string]string{},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: name,
						},
					},
				},
			},
		},
	}

	if resource := manifest.GetFirst(kubeutil.DeploymentV1); resource != nil {
		defaultDeployment = resource.(*appsv1.Deployment)
	}

	defaultDeployment.ObjectMeta = getObjectMeta(defaultDeployment.ObjectMeta, appName, r.Name, r.ResourceTypeName(), *options)
	if defaultDeployment.Spec.Selector == nil {
		defaultDeployment.Spec.Selector = &metav1.LabelSelector{}
	}

	podTemplate := &defaultDeployment.Spec.Template
	if podTemplate.ObjectMeta.Labels == nil {
		podTemplate.ObjectMeta.Labels = map[string]string{}
	}

	if podTemplate.ObjectMeta.Annotations == nil {
		podTemplate.ObjectMeta.Annotations = map[string]string{}
	}

	if len(podTemplate.Spec.Containers) == 0 {
		podTemplate.Spec.Containers = []corev1.Container{}
	}

	found := false
	for _, container := range podTemplate.Spec.Containers {
		if strings.EqualFold(container.Name, name) {
			found = true
			break
		}
	}
	if !found {
		podTemplate.Spec.Containers = append(podTemplate.Spec.Containers, corev1.Container{Name: name})
	}

	return defaultDeployment
}

func getServiceAccountBase(manifest kubeutil.ObjectManifest, appName string, r *datamodel.ContainerResource, options *renderers.RenderOptions) *corev1.ServiceAccount {
	// If the service account has a base manifest, get the service account resource from the base manifest.
	// Otherwise, populate default resources.
	defaultAccount := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
	}

	if resource := manifest.GetFirst(kubeutil.ServiceAccountV1); resource != nil {
		defaultAccount = resource.(*corev1.ServiceAccount)
	}

	defaultAccount.ObjectMeta = getObjectMeta(defaultAccount.ObjectMeta, appName, r.Name, r.ResourceTypeName(), *options)

	return defaultAccount
}
