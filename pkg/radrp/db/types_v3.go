// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

// ApplicationResource represents a Radius application as stored in the database.
type ApplicationResource struct {
	ID              string            `bson:"_id"`
	Type            string            `bson:"type"`
	SubscriptionID  string            `bson:"subscriptionId"`
	ResourceGroup   string            `bson:"resourceGroup"`
	ApplicationName string            `bson:"applicationName"`
	Tags            map[string]string `bson:"tags"`
	Location        string            `bson:"location"`

	// FYI: Applications have no definition.

	Status ApplicationResourceStatus `bson:"status"`
	// FYI: Applications have no provisioning state.
}

type ApplicationResourceStatus = ApplicationStatus

// RadiusResource represents one of the child resources of Application as stored in the database.
type RadiusResource struct {
	ID              string `bson:"_id"`
	Type            string `bson:"type"`
	SubscriptionID  string `bson:"subscriptionId"`
	ResourceGroup   string `bson:"resourceGroup"`
	ApplicationName string `bson:"applicationName"`
	ResourceName    string `bson:"resourceName"`

	Definition map[string]interface{} `bson:"definition"`

	Status            RadiusResourceStatus `bson:"status"`
	ProvisioningState string               `bson:"provisioningState"`
}

type RadiusResourceStatus = ComponentStatus
