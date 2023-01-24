// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
)

// Options is the options to configure LinkRP controller.
type Options struct {
	ctrl.Options

	// DeployProcessor is the deployment processor for LinkRP
	DeployProcessor deployment.DeploymentProcessor
}

const (

	// User defined operation names
	OperationListSecret = "LISTSECRETS"
)

var LinkTypes = []string{
	linkrp.DaprInvokeHttpRoutesResourceType,
	linkrp.DaprPubSubBrokersResourceType,
	linkrp.DaprSecretStoresResourceType,
	linkrp.DaprStateStoresResourceType,
	linkrp.ExtendersResourceType,
	linkrp.MongoDatabasesResourceType,
	linkrp.RabbitMQMessageQueuesResourceType,
	linkrp.RedisCachesResourceType,
	linkrp.SqlDatabasesResourceType,
}
