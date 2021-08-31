// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package health

import (
	"time"

	"github.com/Azure/radius/pkg/health/db"
	"github.com/Azure/radius/pkg/health/model"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/go-logr/logr"
)

// Default values for Health check options
const (
	HealthCheckFrequencyInSecs = 10 * time.Second
)

// Option is an a function that applies a health check option
type HealthCheckOption func(o *healthcontract.HealthCheckOptions)

func getHealthCheckOptions(o *healthcontract.HealthCheckOptions, msgOptions *healthcontract.HealthCheckOptions) {
	// Read incoming message values or apply defaults
	if msgOptions == nil || msgOptions.Interval == 0 {
		o.Interval = HealthCheckFrequencyInSecs
	} else {
		o.Interval = msgOptions.Interval
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
	Logger                      logr.Logger
	DB                          db.RadHealthDB
	ResourceRegistrationChannel chan healthcontract.ResourceHealthRegistrationMessage
	HealthProbeChannel          chan healthcontract.ResourceHealthDataMessage
	WatchHealthChangesChannel   chan healthcontract.ResourceHealthDataMessage
	HealthModel                 model.HealthModel
}
