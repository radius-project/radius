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

package kubeutil

const (
	// DeploymentV1 represents V1 Deployment Kubernetes resource type.
	DeploymentV1 = "apps/v1/deployment"

	// DaemonSetV1 represents V1 DaemonSet Kubernetes resource type.
	ServiceV1 = "core/v1/service"

	// ServiceAccountV1 represents V1 ServiceAccount Kubernetes resource type.
	ServiceAccountV1 = "core/v1/serviceaccount"

	// SecretV1 represents V1 Secret Kubernetes resource type.
	SecretV1 = "core/v1/secret"

	// ConfigMapV1 represents V1 ConfigMap Kubernetes resource type.
	ConfigMapV1 = "core/v1/configmap"
)
