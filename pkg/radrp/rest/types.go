// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rest

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

// Represents the possible HealthState values
const (
	Healthy   = "Healthy"
	Unhealthy = "Unhealthy"
	Degraded  = "Degraded"
)

// ApplicationStatus represents the status of the overall Radius Application
type ApplicationStatus struct {
	ProvisioningState        string `json:"provisioningState"`
	ProvisioningErrorDetails string `json:"provisioningErrorDetails"`
	HealthState              string `json:"healthState"`
	HealthErrorDetails       string `json:"healthErrorDetails"`
}

// OutputResource represents the output of rendering a resource
type OutputResource struct {
	LocalID            string                         `json:"localID"`
	Managed            bool                           `json:"managed"`
	ResourceKind       string                         `json:"resourceKind"`
	OutputResourceType string                         `json:"outputResourceType"`
	OutputResourceInfo resourcemodel.ResourceIdentity `json:"outputResourceInfo"`
	Status             OutputResourceStatus           `json:"status"`
}

// OutputResourceStatus represents the status of the Output Resource
type OutputResourceStatus struct {
	ProvisioningState        string    `json:"provisioningState"`
	ProvisioningErrorDetails string    `json:"provisioningErrorDetails"`
	HealthState              string    `json:"healthState"`
	HealthErrorDetails       string    `json:"healthErrorDetails"`
	Replicas                 []Replica `json:"replicas,omitempty"`
}

// ComponentStatus represents the status of the Radius Component
type ComponentStatus struct {
	ProvisioningState        string           `json:"provisioningState"`
	ProvisioningErrorDetails string           `json:"provisioningErrorDetails"`
	HealthState              string           `json:"healthState"`
	HealthErrorDetails       string           `json:"healthErrorDetails"`
	OutputResources          []OutputResource `json:"outputResources,omitempty"`
}

// Replica represents an individual instance of a resource (Azure/K8s)
type Replica struct {
	ID     string        `json:"id"`
	Status ReplicaStatus `json:"status"`
}

// ReplicaStatus represents the status of a replica
type ReplicaStatus struct {
	ProvisioningState        string `json:"provisioningState"`
	ProvisioningErrorDetails string `json:"provisioningErrorDetails"`
	HealthState              string `json:"healthState"`
	HealthErrorDetails       string `json:"healthErrorDetails"`
}

// See: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/Addendum.md#asynchronous-operations
type OperationStatus string

const (
	// Terminal states
	SuccededStatus OperationStatus = "Succeeded"
	FailedStatus   OperationStatus = "Failed"
	CanceledStatus OperationStatus = "Canceled"

	// RP-defined statuses are used for non-terminal states
	DeployingStatus OperationStatus = "Deploying"
	DeletingStatus  OperationStatus = "Deleting"
)

func IsTeminalStatus(status OperationStatus) bool {
	return status == SuccededStatus || status == FailedStatus || status == CanceledStatus
}
