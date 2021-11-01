// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

import (
	"github.com/Azure/radius/pkg/resourcemodel"
)

// Represents the possible ProvisioningState values
const (
	NotProvisioned = "NotProvisioned"
	Provisioning   = "Provisioning"
	Provisioned    = "Provisioned"
	Failed         = "Failed"
)

// OutputResource represents an output resource comprising a Radius resource.
type OutputResource struct {
	LocalID string `bson:"id"`

	// ResourceKind specifies the 'kind' used to look up the resource handler for processing.
	ResourceKind string `bson:"resourceKind"`

	// Identity specifies the identity of the resource in the underlying platform.
	Identity resourcemodel.ResourceIdentity `bson:"identity"`
	Managed  bool                           `bson:"managed"`

	// We persist properties returned from the resource handler for later use when
	// processing the same resource again.
	//
	// This is an old pattern that we're trying to migrate away from because it requires
	// a resource handler per-resource type. In general any per-resource type processing
	// should be done in the renderer.
	PersistedProperties map[string]string    `bson:"persistedProperties"`
	Status              OutputResourceStatus `bson:"status"`
}

// OutputResourceStatus represents the status of the Output Resource
type OutputResourceStatus struct {
	ProvisioningState        string    `bson:"provisioningState"`
	ProvisioningErrorDetails string    `bson:"provisioningErrorDetails"`
	HealthState              string    `bson:"healthState"`
	HealthStateErrorDetails  string    `bson:"healthStateErrorDetails"`
	Replicas                 []Replica `bson:"replicas,omitempty" structs:"-"` // Ignore stateful property during serialization
}

// Replica represents an individual instance of a resource (Azure/K8s)
type Replica struct {
	ID     string
	Status ReplicaStatus `bson:"status"`
}

// ReplicaStatus represents the status of a replica
type ReplicaStatus struct {
	ProvisioningState string `bson:"provisioningState"`
	HealthState       string `bson:"healthState"`
}
