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
type HealthHandler interface {
	GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions
}
