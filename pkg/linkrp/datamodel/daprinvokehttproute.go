// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// DaprInvokeHttpRoute represents DaprInvokeHttpRoute link resource.
type DaprInvokeHttpRoute struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprInvokeHttpRouteProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (r *DaprInvokeHttpRoute) Transform(outputResources []outputresource.OutputResource, computedValues map[string]any, secretValues map[string]rp.SecretValueReference) error {
	r.Properties.Status.OutputResources = outputResources
	r.ComputedValues = computedValues
	r.SecretValues = secretValues
	if appId, ok := computedValues[linkrp.AppIDKey].(string); ok {
		r.Properties.AppId = appId
	}

	return nil
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *DaprInvokeHttpRoute) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the output resources array.
func (r *DaprInvokeHttpRoute) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *DaprInvokeHttpRoute) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// ComputedValues returns the computed values on the link.
func (r *DaprInvokeHttpRoute) GetComputedValues() map[string]any {
	return r.LinkMetadata.ComputedValues
}

// SecretValues returns the secret values for the link.
func (r *DaprInvokeHttpRoute) GetSecretValues() map[string]rp.SecretValueReference {
	return r.LinkMetadata.SecretValues
}

// RecipeData returns the recipe data for the link.
func (r *DaprInvokeHttpRoute) GetRecipeData() RecipeData {
	return r.LinkMetadata.RecipeData
}

func (httpRoute *DaprInvokeHttpRoute) ResourceTypeName() string {
	return linkrp.DaprInvokeHttpRoutesResourceType
}

// DaprInvokeHttpRouteProperties represents the properties of DaprInvokeHttpRoute resource.
type DaprInvokeHttpRouteProperties struct {
	rpv1.BasicResourceProperties
	Recipe linkrp.LinkRecipe `json:"recipe,omitempty"`
	AppId  string            `json:"appId"`
}
