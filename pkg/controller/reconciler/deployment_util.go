/*
Copyright 2023.

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

package reconciler

import (
	"github.com/radius-project/radius/pkg/to"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// addSecretReference adds a secret reference to the deployment. Returns true if the secret was added, and false if it already exists.
//
// This function is idempotent and will not add the secret reference if it already exists.
func addSecretReference(deployment *appsv1.Deployment, secretName string) bool {
	// For now we're just interested in the first container.
	container := &deployment.Spec.Template.Spec.Containers[0]

	index := -1
	for i := range deployment.Spec.Template.Spec.Containers[0].EnvFrom {
		if container.EnvFrom[i].SecretRef != nil && container.EnvFrom[i].SecretRef.Name == secretName {
			index = i
			break
		}
	}

	if index != -1 {
		return false // Already present
	}

	from := corev1.EnvFromSource{
		SecretRef: &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
			Optional:             to.Ptr(false),
		},
	}

	container.EnvFrom = append(container.EnvFrom, from)
	return true
}

// removeSecretReference removes the secret reference from the deployment. Returns true if the secret was removed, and false if it was not found.
func removeSecretReference(deployment *appsv1.Deployment, secretName string) bool {

	// For now we're just interested in the first container.
	container := &deployment.Spec.Template.Spec.Containers[0]

	index := -1
	for i := range deployment.Spec.Template.Spec.Containers[0].EnvFrom {
		if container.EnvFrom[i].SecretRef != nil && container.EnvFrom[i].SecretRef.Name == secretName {
			index = i
			break
		}
	}

	if index == -1 {
		return false
	}

	// Remove the secret from the deployment.
	container.EnvFrom = append(container.EnvFrom[0:index], container.EnvFrom[index+1:]...)
	return true
}
