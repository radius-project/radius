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

package radius

import "github.com/radius-project/radius/pkg/ucp/resources"

const (
	// PlaneTypeRadius defines the type name of the Radius plane.
	PlaneTypeRadius = "radius"

	// ScopeResourceGroups is the scope for a Radius Resource Group.
	ScopeResourceGroups = "resourcegroups"

	// NamespaceApplicationsCore defines the namespace for the Radius Applications.Core resource provider.
	NamespaceApplicationsCore = "Applications.Core"

	// NamespaceApplicationsDatastores defines the namespace for the Radius Applications.Datastores resource provider.
	NamespaceApplicationsDatastores = "Applications.Datastores"

	// NamespaceApplicationsDapr defines the namespace for the Radius Applications.Dapr resource provider.
	NamespaceApplicationsDapr = "Applications.Dapr"

	// NamespaceApplicationsMessaging defines the namespace for the Radius Applications.Messaging resource provider.
	NamespaceApplicationsMessaging = "Applications.Messaging"
)

// IsRadiusResource checks if the given ID represents a resource type, and is defined in the Radius plane.
func IsRadiusResource(id resources.ID) bool {
	return id.FindScope("radius") != "" && id.IsResource()
}
