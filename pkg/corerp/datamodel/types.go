// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

// ProvisioningStates is the state of resource.
type ProvisioningStates string

const (
	ProvisioningStateNone      ProvisioningStates = "None"
	ProvisioningStateUpdating  ProvisioningStates = "Updating"
	ProvisioningStateDeleting  ProvisioningStates = "Deleting"
	ProvisioningStateAccepted  ProvisioningStates = "Accepted"
	ProvisioningStateSucceeded ProvisioningStates = "Succeeded"
	ProvisioningStateFailed    ProvisioningStates = "Failed"
	ProvisioningStateCanceled  ProvisioningStates = "Canceled"
)
