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
	"testing"

	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func Test_addSecretReference_AlreadyPresent(t *testing.T) {
	expected := []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "secret"}},
		},
		{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "another"}},
		},
		{
			ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "config"}},
		},
	}

	deployment := makeDeployment(types.NamespacedName{})
	deployment.Spec.Template.Spec.Containers[0].EnvFrom = expected

	result := addSecretReference(deployment, "secret")
	require.False(t, result)
	require.Equal(t, expected, deployment.Spec.Template.Spec.Containers[0].EnvFrom)
}

func Test_addSecretReference_ReferenceAdded(t *testing.T) {
	expected := []corev1.EnvFromSource{

		{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "another"}},
		},
		{
			ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "config"}},
		},
		{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "secret"}, Optional: to.Ptr(false)},
		},
	}

	deployment := makeDeployment(types.NamespacedName{})
	deployment.Spec.Template.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "another"}},
		},
		{
			ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "config"}},
		},
	}

	result := addSecretReference(deployment, "secret")
	require.True(t, result)
	require.Equal(t, expected, deployment.Spec.Template.Spec.Containers[0].EnvFrom)
}

func Test_removeSecretReference_AlreadyRemoved(t *testing.T) {
	expected := []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "another"}},
		},
		{
			ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "config"}},
		},
	}

	deployment := makeDeployment(types.NamespacedName{})
	deployment.Spec.Template.Spec.Containers[0].EnvFrom = expected

	result := removeSecretReference(deployment, "secret")
	require.False(t, result)
	require.Equal(t, expected, deployment.Spec.Template.Spec.Containers[0].EnvFrom)
}

func Test_removeSecretReference_ReferenceRemoved(t *testing.T) {
	expected := []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "another"}},
		},
		{
			ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "config"}},
		},
	}

	deployment := makeDeployment(types.NamespacedName{})
	deployment.Spec.Template.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "secret"}},
		},
		{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "another"}},
		},
		{
			ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "config"}},
		},
	}

	result := removeSecretReference(deployment, "secret")
	require.True(t, result)
	require.Equal(t, expected, deployment.Spec.Template.Spec.Containers[0].EnvFrom)
}
