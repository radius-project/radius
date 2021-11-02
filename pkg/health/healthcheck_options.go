// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package health

import (
	"time"

	"github.com/Azure/radius/pkg/health/db"
	"github.com/Azure/radius/pkg/health/handlers"
	"github.com/Azure/radius/pkg/health/model"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/go-logr/logr"
)

// Default values for Health check options
const (
	DefaultHealthCheckFrequencyInSecs = 10 * time.Second
	// DefaultForceHealthStateUpdateInterval is the interval after which a health state change notification is sent to the RP
	// even if there are no changes. This is for increased robustness in case the RP has missed earlier notifications
	DefaultForceHealthStateUpdateInterval = time.Second * 30
)

// Option is an a function that applies a health check option
type HealthCheckOption func(o *healthcontract.HealthCheckOptions)

func getHealthCheckOptions(o *healthcontract.HealthCheckOptions, msgOptions *healthcontract.HealthCheckOptions) {
	// Read incoming message values or apply defaults
	if msgOptions == nil || msgOptions.Interval == 0 {
		o.Interval = DefaultHealthCheckFrequencyInSecs
	} else {
		o.Interval = msgOptions.Interval
	}

	if msgOptions == nil || msgOptions.ForcedUpdateInterval == 0 {
		o.ForcedUpdateInterval = DefaultForceHealthStateUpdateInterval
	} else {
		o.ForcedUpdateInterval = msgOptions.ForcedUpdateInterval
	}
}

// WithInterval sets the interval for the health check
func WithInterval(interval time.Duration) HealthCheckOption {
	return func(o *healthcontract.HealthCheckOptions) {
		o.Interval = interval
	}
}

// MonitorOptions are the options that are passed in to the health service
type MonitorOptions struct {
	Logger logr.Logger
	DB     db.RadHealthDB
	// ResourceRegistrationChannel is used to receive registration/unregistration messages
	ResourceRegistrationChannel chan healthcontract.ResourceHealthRegistrationMessage
	// HealthProbeChannel is used to send health updates for resources to the RP
	HealthProbeChannel chan healthcontract.ResourceHealthDataMessage
	// WatchHealthChangesChannel is used to receive health change notifications from push mode watchers
	WatchHealthChangesChannel chan handlers.HealthState
	HealthModel               model.HealthModel
}
