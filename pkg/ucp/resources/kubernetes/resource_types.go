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

package kubernetes

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	// KindDeployment is the kind of a Kubernetes Deployment.
	KindDeployment = "Deployment"
	// ResourceTypeDeployment is the resource type of a Kubernetes Deployment.
	ResourceTypeDeployment = "apps/Deployment"
	// KindSecret is the kind of a Kubernetes Secret.
	KindSecret = "Secret"
	// ResourceTypeSecret is the resource type of a Kubernetes Secret.
	ResourceTypeSecret = "core/Secret"
	// KindService is the kind of a Kubernetes Service.
	KindService = "Service"
	// ResourceTypeService is the resource type of a Kubernetes Service.
	ResourceTypeService = "core/Service"
	// KindServiceAccount is the kind of a Kubernetes ServiceAccount.
	KindServiceAccount = "ServiceAccount"
	// ResourceTypeServiceAccount is the resource type of a Kubernetes ServiceAccount.
	ResourceTypeServiceAccount = "core/ServiceAccount"
	// KindRole is the kind of a Kubernetes Role.
	KindRole = "Role"
	// ResourceTypeRole is the resource type of a Kubernetes Role.
	ResourceTypeRole = "rbac.authorization.k8s.io/Role"
	// KindRoleBinding is the kind of a Kubernetes RoleBinding.
	KindRoleBinding = "RoleBinding"
	// ResourceTypeRoleBinding is the resource type of a Kubernetes RoleBinding.
	ResourceTypeRoleBinding = "rbac.authorization.k8s.io/RoleBinding"
	// KindSecretProviderClass is the kind of a Kubernetes SecretProviderClass.
	KindSecretProviderClass = "SecretProviderClass"
	// ResourceTypeSecretProviderClass is the resource type of a Kubernetes SecretProviderClass.
	ResourceTypeSecretProviderClass = "secrets-store.csi.x-k8s.io/SecretProviderClass"

	// KindContourHTTPProxy is the kind of a Contour HTTPProxy.
	KindContourHTTPProxy = "HTTPProxy"
	// ResourceTypeContourHTTPProxy is the resource type of a Contour HTTPProxy.
	ResourceTypeContourHTTPProxy = "projectcontour.io/HTTPProxy"

	// ResourceTypeDaprComponent is the resource type of a Dapr component.
	ResourceTypeDaprComponent = "dapr.io/Component"
)

// ResourceTypeFromGVK returns the resource type of a Kubernetes resource given its group, version, and kind.
func ResourceTypeFromGVK(gvk schema.GroupVersionKind) string {
	group := gvk.Group
	if group == "" {
		group = "core"
	}

	return group + "/" + gvk.Kind
}
