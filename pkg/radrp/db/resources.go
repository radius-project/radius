// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

// RadiusResource represents one of the child resources of Application as stored in the database.
type RadiusResource struct {
	ID              string `bson:"_id"`
	Type            string `bson:"type"`
	SubscriptionID  string `bson:"subscriptionId"`
	ResourceGroup   string `bson:"resourceGroup"`
	ApplicationName string `bson:"applicationName"`
	ResourceName    string `bson:"resourceName"`

	Definition     map[string]interface{} `bson:"definition"`
	ComputedValues map[string]interface{} `bson:"computedValues"`

	// NOTE: this is not part of the output of the RP - this is internal tracking
	// for how we can look up values that do not store.
	SecretValues map[string]SecretValueReference `bson:"secretValues"`

	Status            RadiusResourceStatus `bson:"status"`
	ProvisioningState string               `bson:"provisioningState"`
}

type RadiusResourceStatus struct {
	ProvisioningState string           `bson:"provisioningState"`
	HealthState       string           `bson:"healthState"`
	OutputResources   []OutputResource `bson:"outputResources,omitempty" structs:"-"` // Ignore stateful property during serialization
}

// see renderers.SecretValueReference for description
type SecretValueReference struct {
	LocalID       string `bson:"localId"`
	Action        string `bson:"action,omitempty"`
	ValueSelector string `bson:"valueSelector"`
	Transformer   string `bson:"transformer,omitempty"`
}
