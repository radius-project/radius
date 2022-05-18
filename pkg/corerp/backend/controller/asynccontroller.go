// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import "context"

// AsyncControllerInterface is an interface to implement async operation controller.
type AsyncControllerInterface interface {
	Run(ctx context.Context) error
}
