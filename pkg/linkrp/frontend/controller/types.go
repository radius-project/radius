// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
)

// Options is the options to configure LinkRP controller.
type Options struct {
	ctrl.Options

	// DeployProcessor is the deployment processor for LinkRP
	DeployProcessor deployment.DeploymentProcessor
}
