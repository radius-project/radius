// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/Azure/radius/pkg/health/handleroptions"
	"github.com/Azure/radius/pkg/healthcontract"
)

// HealthHandler interface defines the health check methods that every resource kind will implement
//go:generate mockgen -destination=./mock_healthhandler.go -package=handlers -self_package github.com/Azure/radius/pkg/health/handlers github.com/Azure/radius/pkg/health/handlers HealthHandler

type HealthHandler interface {
	GetHealthState(ctx context.Context, resourceInfo healthcontract.ResourceInfo, options handleroptions.Options) healthcontract.ResourceHealthDataMessage
}
