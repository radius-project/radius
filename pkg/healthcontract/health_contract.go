// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package healthcontract

import (
	"encoding/json"
	"time"
)

// Possible action values for a RegistrationMessage
const (
	ActionRegister   = "Register"
	ActionUnregister = "Unregister"
)

// Possible values for HealthState
const (
	HealthStateHealthy   = "Healthy"
	HealthStateUnhealthy = "Unhealthy"
)

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

// HealthIDKey is the key used by all resource types to represent the id used to register with HealthService
const HealthIDKey = "healthid"

// ResourceIDKey is the key used by all resource types to return the actual resource to be tracked by the HealthService
const ResourceIDKey = "resourceid"

// ResourceInfo includes the resource information that is required to perform its health check
type ResourceInfo struct {
	// Identifier used to register a resource with the HealthService and is unique across Radius applications/components
	HealthID string
	// Identifier actually used to query health e.g. ARM Resource ID
	ResourceID   string
	ResourceKind string
}

// ResourceDetails represents the information needed to uniquely identify an output resource across applications/components
type ResourceDetails struct {
	ResourceID     string
	ResourceKind   string
	ApplicationID  string
	ComponentID    string
	SubscriptionID string
	ResourceGroup  string
}

// ParseHealthID parses a string healthID and returns a ResourceDetails data structure
func ParseHealthID(id string) ResourceDetails {
	rd := ResourceDetails{}
	err := json.Unmarshal([]byte(id), &rd)
	if err != nil {
		return ResourceDetails{}
	}
	return rd
}

// GetHealthID returns a unique identifier to identify the output resource in the HealthService
func (r ResourceDetails) GetHealthID() string {
	bytes, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	return string(bytes)
}

// ResourceHealthRegistrationMessage used by callers to register/de-register a resource from the health monitoring service
type ResourceHealthRegistrationMessage struct {
	Action       string
	ResourceInfo ResourceInfo
	Options      HealthCheckOptions
}

// ResourceHealthDataMessage is the message sent by individual resources to communicate the current health state
type ResourceHealthDataMessage struct {
	Resource                ResourceInfo
	HealthState             string
	HealthStateErrorDetails string
}
