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

// BaseController is the base controller for api controller.
type BaseController struct {
	DBProvider db.RadrpDB
	JobEngine  deployment.DeploymentProcessor
}

// ControllerInterface is the interface of each operation controller.
type ControllerInterface interface {
	Run(ctx context.Context, req *http.Request) (rest.Response, error)
}
