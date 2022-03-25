// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
)

// BaseController is the base controller for api controller.
type BaseController struct {
	db     db.RadrpDB
	deploy deployment.DeploymentProcessor

	// completions is used to signal the completion of asynchronous processing. This is use for tests
	// So we can avoid panics happening when the test is finished.
	//
	// DO NOT use this to implement product functionality, this is a hook for testing.
	completions chan<- struct{}
	scheme      string
}
