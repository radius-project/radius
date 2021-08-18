// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/Azure/radius/pkg/healthcontract"
)

// HealthHandler interface defines the health check methods that every resource kind will implement
type HealthHandler interface {
	GetHealthState(ctx context.Context, resourceInfo healthcontract.ResourceInfo) healthcontract.ResourceHealthDataMessage
}
