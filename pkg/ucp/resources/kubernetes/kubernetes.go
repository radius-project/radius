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

import (
	"strings"

	"github.com/radius-project/radius/pkg/ucp/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// PlaneTypeKubernetes defines the type name of the Kubernetes plane.
	PlaneTypeKubernetes = "kubernetes"

	// PlaneNameTODO is the name of the Kubernetes plane to use when the plane name is not known.
	// This is similar to context.TODO() in the Go standard library. When we support multiple kubernetes
	// clusters in a single Radius instance, we will need to remove this and replace all occurrences.
	PlaneNameTODO = "local"

	// ScopeTypeNamespaces defines the type name of the Kubernetes namespace scope.
	ScopeNamespaces = "namespaces"
)

// ToParts returns the component parts of the given UCP resource ID.
func ToParts(id resources.ID) (group, kind, namespace, name string) {
	namespace = id.FindScope(ScopeNamespaces)
	name = id.Name()
	group = id.ProviderNamespace()
	if group == "core" {
		group = ""
	}
	_, kind, _ = strings.Cut(id.Type(), "/")
	return group, kind, namespace, name
}

// IDFromMeta returns the UCP resource ID for the given Kubernetes object specified by its GroupVersionKind
// and ObjectMeta.
func IDFromMeta(planeName string, gvk schema.GroupVersionKind, objectMeta metav1.ObjectMeta) resources.ID {
	return IDFromParts(planeName, gvk.Group, gvk.Kind, objectMeta.Namespace, objectMeta.Name)
}

// IDFromParts returns the UCP resource ID for the given Kubernetes object specified by its component parts.
func IDFromParts(planeName string, group string, kind string, namespace string, name string) resources.ID {
	if group == "" {
		group = "core"
	}

	scopes := []resources.ScopeSegment{
		{
			Type: PlaneTypeKubernetes,
			Name: planeName,
		},
	}

	if namespace != "" {
		scopes = append(scopes, resources.ScopeSegment{
			Type: ScopeNamespaces,
			Name: namespace,
		})
	}

	types := []resources.TypeSegment{
		{
			Type: group + "/" + kind,
			Name: name,
		},
	}

	return resources.MustParse(resources.MakeUCPID(scopes, types, nil))
}
