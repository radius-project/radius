package common

// ProvisioningStates is the state of resource.
type ProvisioningStates string

const (
	ProvisioningStateUpdating  ProvisioningStates = "Updating"
	ProvisioningStateDeleting  ProvisioningStates = "Deleting"
	ProvisioningStateAccepted  ProvisioningStates = "Accepted"
	ProvisioningStateSucceeded ProvisioningStates = "Succeeded"
	ProvisioningStateFailed    ProvisioningStates = "Failed"
	ProvisioningStateCanceled  ProvisioningStates = "Canceled"
)

// ResourceTags is the type of ARM tags.
type ResourceTags map[string]string
