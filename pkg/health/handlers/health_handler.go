// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
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

// HealthHandler interface defines the health check methods that every resource kind will implement
//go:generate mockgen -destination=./mock_healthhandler.go -package=handlers -self_package github.com/Azure/radius/pkg/health/handlers github.com/Azure/radius/pkg/health/handlers HealthHandler

type HealthHandler interface {
	GetHealthState(ctx context.Context, resourceInfo healthcontract.ResourceInfo, options Options) healthcontract.ResourceHealthDataMessage
}
