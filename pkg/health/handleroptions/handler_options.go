// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handleroptions

import (
	"time"

	"github.com/Azure/radius/pkg/healthcontract"
)

// Possible values for HealthHandlerMode
// Kubernetes supports Push mode where K8s notifies the health service upon changes to the resource
// Azure supports Pull mode where the health service needs to actively poll for the health of the resource
const (
	HealthHandlerModePush = "Push"
	HealthHandlerModePull = "Pull"
)

type Options struct {
	// Poll interval as specified by the resource
	Interval time.Duration
	// Channel to receive a notification to stop watching the resource
	StopChannel chan struct{}
	// Channel to communicate detected changes by the push mode watcher on to the health service
	WatchHealthChangesChannel chan healthcontract.ResourceHealthDataMessage
}
