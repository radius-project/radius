package recipecontext

import (
	"encoding/json"
	"errors"

	"github.com/project-radius/radius/pkg/recipes"
)

var (
	ErrRecipeConversion = errors.New("fails to convert recipe context to map structure")
)

// RecipeContext Recipe template authors can leverage the RecipeContext parameter to access Link properties to
// generate name and properties that are unique for the Link calling the recipe.
type RecipeContext struct {
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

func (r *RecipeContext) ToMap() (map[string]any, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, ErrRecipeConversion
	}
	m := map[string]any{}
	err = json.Unmarshal(data, &m)
	if err != nil {
		return nil, ErrRecipeConversion
	}
	return m, nil
}

// Resource contains the information needed to deploy a recipe.
// In the case the resource is a Link, it represents the Link's id, name and type.
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
