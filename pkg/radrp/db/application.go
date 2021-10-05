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

type ApplicationResourceStatus struct {
	ProvisioningState        string `bson:"provisioningState"`
	ProvisioningErrorDetails string `bson:"provisioningErrorDetails"`
	HealthState              string `bson:"healthState"`
	HealthErrorDetails       string `bson:"healthErrorDetails"`
}
