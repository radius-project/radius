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

package recipecontext

import (
	"github.com/radius-project/radius/pkg/recipes"
)

const (
	// RecipeContextParamKey represents the key for the recipe context object parameter.
	RecipeContextParamKey = "context"
)

// Context represents the context information which accesses portable resource properties. Recipe template authors
// can leverage the RecipeContext parameter to access portable resource properties to generate name and properties
// that are unique for the portable resource calling the recipe.
type Context struct {
	// Resource represents the resource information of the deploying recipe resource.
	Resource Resource `json:"resource,omitempty"`
	// Application represents environment resource information.
	Application ResourceInfo `json:"application,omitempty"`
	// Environment represents environment resource information.
	Environment ResourceInfo `json:"environment,omitempty"`
	// Runtime represents Kubernetes Runtime configuration.
	Runtime recipes.RuntimeConfiguration `json:"runtime,omitempty"`
	// Azure represents Azure provider scope.
	Azure *ProviderAzure `json:"azure,omitempty"`
	// AWS represents AWS provider scope.
	AWS *ProviderAWS `json:"aws,omitempty"`
}

// Resource contains the information needed to deploy a recipe.
// In the case the resource is a portable resource, it represents the resource's id, name and type.
type Resource struct {
	// ResourceInfo represents name and id of the resource
	ResourceInfo
	// Type represents the resource type, this will be a namespace/type combo. Ex. Applications.Core/Environment
	Type string `json:"type"`
}

// ResourceInfo represents name and id of the resource
type ResourceInfo struct {
	// Name represents the resource name.
	Name string `json:"name"`
	// ID represents fully qualified resource id.
	ID string `json:"id"`
}

// ProviderAzure contains Azure provider scope for recipe context.
type ProviderAzure struct {
	// ResourceGroup represents the resource group information.
	ResourceGroup AzureResourceGroup `json:"resourceGroup,omitempty"`
	// Subscription represents the subscription information.
	Subscription AzureSubscription `json:"subscription,omitempty"`
}

// AzureResourceGroup contains Azure Resource Group provider information.
type AzureResourceGroup struct {
	// Name represents the resource name.
	Name string `json:"name"`
	// ID represents fully qualified resource group name.
	ID string `json:"id"`
}

// AzureSubscription contains Azure Subscription provider information.
type AzureSubscription struct {
	// SubscriptionID represents the id of subscription.
	SubscriptionID string `json:"subscriptionId"`
	// ID represents fully qualified subscription id.
	ID string `json:"id"`
}

// ProviderAWS contains AWS Account provider scope for recipe context.
type ProviderAWS struct {
	// Region represents the region of the AWS account.
	Region string `json:"region"`
	// Account represents the account id of the AWS account.
	Account string `json:"account"`
}
