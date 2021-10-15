// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package healthcontract

import (
	"time"

	"github.com/Azure/radius/pkg/resourcemodel"
)

// ChannelBufferSize defines the buffer size for health registration channel
const ChannelBufferSize = 100

// Possible action values for a RegistrationMessage
const (
	ActionRegister   = "Register"
	ActionUnregister = "Unregister"
)

// Possible values for HealthState for internal representation of the health state
// We will represent the health internally in a manner that gives us the full picture of what is happening
// This is not representative of the values shown to the user
const (
	HealthStateUnknown       = "Unknown" // Health reporting is implemented but the state is not yet known. This is different from the case where health reporting is not supported.
	HealthStateHealthy       = "Healthy"
	HealthStateUnhealthy     = "Unhealthy"
	HealthStateDegraded      = "Degraded"      // Functionality is working but some resources are unhealthy
	HealthStateNotSupported  = "NotSupported"  // Health reporting has not yet been implemented
	HealthStateNotApplicable = "NotApplicable" // Health as a concept does not apply to this resource eg: Secrets
	HealthStateError         = "Error"         // See unexpected errors
)

// Translation of internal representation of health state to user facing values
var InternalToUserHealthStateTranslation = map[string]string{
	HealthStateUnknown:       HealthStateUnhealthy,
	HealthStateHealthy:       HealthStateHealthy,
	HealthStateUnhealthy:     HealthStateUnhealthy,
	HealthStateDegraded:      HealthStateDegraded,
	HealthStateNotSupported:  "",
	HealthStateNotApplicable: HealthStateHealthy,
	HealthStateError:         HealthStateUnhealthy,
}

// HealthCheckOptions defines the options available for performing health check of a resource
type HealthCheckOptions struct {
	Interval time.Duration
}

// HealthChannels defines the interface to connect to the health service
type HealthChannels struct {
	// ResourceRegistrationWithHealthChannel is the channel on which RP registers with the health service (RP -> HealthService)
	ResourceRegistrationWithHealthChannel chan ResourceHealthRegistrationMessage
	// HealthToRPNotificationChannel is the channel on which the HealthService sends health state change notifications to the RP (HealthService -> RP)
	HealthToRPNotificationChannel chan ResourceHealthDataMessage
}

func NewHealthChannels() HealthChannels {
	rrc := make(chan ResourceHealthRegistrationMessage, ChannelBufferSize)
	hpc := make(chan ResourceHealthDataMessage, ChannelBufferSize)
	return HealthChannels{
		ResourceRegistrationWithHealthChannel: rrc,
		HealthToRPNotificationChannel:         hpc,
	}
}

// HealthResource represents the information needed to register and unregister an application with the health tracking system.
type HealthResource struct {
	// ResourceKind is the handler kind of the resource.
	ResourceKind string

	// Identity is the identity of the resource in its underlying platform.
	Identity resourcemodel.ResourceIdentity

	// RadiusResourceID is the resource ID of the Radius Resource that 'owns' this output resource.
	RadiusResourceID string
}

// ResourceHealthRegistrationMessage used by callers to register/de-register a resource from the health monitoring service
type ResourceHealthRegistrationMessage struct {
	Action   string
	Resource HealthResource
	Options  HealthCheckOptions
}

// ResourceHealthDataMessage is the message sent by individual resources to communicate the current health state
type ResourceHealthDataMessage struct {
	Resource                HealthResource
	HealthState             string
	HealthStateErrorDetails string
}
