// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

// BaseController is the base operation controller.
type BaseController struct {
	// TODO: db.RadrpDB and deployment.DeploymentProcessor will be replaced with new implementation.
	DBProvider db.RadrpDB
	JobEngine  deployment.DeploymentProcessor
}

// ControllerInterface is the interface of each operation controller.
type ControllerInterface interface {
	// Run executes operation controller implementation.
	Run(ctx context.Context, req *http.Request) (rest.Response, error)
}
