// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/store"
)

// ControllerInterface is an interface of each operation controller.
type ControllerInterface interface {
	// Run executes the operation.
	Run(ctx context.Context, req *http.Request) (rest.Response, error)
}

// BaseController is the base operation controller.
type BaseController struct {
	// TODO: db.RadrpDB and deployment.DeploymentProcessor will be replaced with new implementation.
	DBClient  store.StorageClient
	JobEngine deployment.DeploymentProcessor
}
