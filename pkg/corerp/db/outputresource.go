// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

import (
	"github.com/project-radius/radius/pkg/resourcemodel"
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
	LocalID string `json:"id"`

	// ResourceType specifies the 'type' and 'provider' used to look up the resource handler for processing.
	ResourceType resourcemodel.ResourceType `json:"resourceType"`

	// Identity specifies the identity of the resource in the underlying platform.
	Identity resourcemodel.ResourceIdentity `json:"identity"`

	// We persist properties returned from the resource handler for later use when
	// processing the same resource again.
	//
	// This is an old pattern that we're trying to migrate away from because it requires
	// a resource handler per-resource type. In general any per-resource type processing
	// should be done in the renderer.
	PersistedProperties map[string]string    `json:"persistedProperties"`
	Status              OutputResourceStatus `json:"status"`
}

// OutputResourceStatus represents the status of the Output Resource
type OutputResourceStatus struct {
	ProvisioningState        string    `json:"provisioningState"`
	ProvisioningErrorDetails string    `json:"provisioningErrorDetails"`
	Replicas                 []Replica `json:"replicas,omitempty" structs:"-"` // Ignore stateful property during serialization
}

// Replica represents an individual instance of a resource (Azure/K8s)
type Replica struct {
	ID     string
	Status ReplicaStatus `json:"status"`
}

// ReplicaStatus represents the status of a replica
type ReplicaStatus struct {
	ProvisioningState string `json:"provisioningState"`
}
