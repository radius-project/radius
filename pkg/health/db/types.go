// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

// This package defines the data types that we store in the db - these are different from
// what we serialize over the wire.

// Represents the possible ProvisioningState values
const (
	NotProvisioned = "NotProvisioned"
	Provisioning   = "Provisioning"
	Provisioned    = "Provisioned"
	Failed         = "Failed"
)

// Represents the possible HealthState values
const (
	Healthy   = "Healthy"
	Unhealthy = "Unhealthy"
	Degraded  = "Degraded"
)

// OutputResourceHealthEvent is the data stored per output resource when the health check ticker triggers
type OutputResourceHealthEvent struct {
	Timestamp               string `bson:"timestamp"`
	ResourceID              string `bson:"resourceID"`
	HealthState             string `bson:"healthState"`
	HealthStateErrorDetails string `bson:"healthStateErrorDetails"`
}
