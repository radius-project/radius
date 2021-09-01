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
//go:generate mockgen -destination=./mock_health_handler.go -package=handlers -self_package github.com/Azure/radius/pkg/handlers github.com/Azure/radius/pkg/handlers HealthHandler
type HealthHandler interface {
	GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions
}
