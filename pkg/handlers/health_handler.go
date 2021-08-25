// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/Azure/radius/pkg/healthcontract"
)

// HealthHandler interface defines the methods that every output resource will implement for registering/unregistering with health service
//go:generate mockgen -destination=../../mocks/mockhandlers/mock_health_handler.go -package=mockhandlers github.com/Azure/radius/pkg/handlers HealthHandler
type HealthHandler interface {
	GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions
}
