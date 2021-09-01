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
const (
	HealthHandlerModePush = "Push"
	HealthHandlerModePull = "Pull"
)

type Options struct {
	Interval                  time.Duration
	StopCh                    chan struct{}
	WatchHealthChangesChannel chan healthcontract.ResourceHealthDataMessage
}
