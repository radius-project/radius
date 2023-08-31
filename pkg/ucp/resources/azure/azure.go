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

package azure

import "github.com/radius-project/radius/pkg/ucp/resources"

const (
	// PlaneTypeAzure defines the type name of the Azure plane.
	PlaneTypeAzure = "azure"

	// ScopeSubscriptions is the scope for an Azure subscription ID.
	ScopeSubscriptions = "subscriptions"
	// ScopeResourceGroups is the scope for an Azure Resource Group.
	ScopeResourceGroups = "resourcegroups"
)

// IsAzureResource returns true if the given resource ID is an Azure resource.
func IsAzureResource(id resources.ID) bool {
	return (id.FindScope(ScopeSubscriptions) != "" || id.FindScope(PlaneTypeAzure) != "") && id.IsResource()
}
