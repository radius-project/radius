// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/health/handlers"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/resourcemodel"
)

type HealthModel interface {
	LookupHandler(ctx context.Context, registerMsg healthcontract.ResourceHealthRegistrationMessage) (handlers.HealthHandler, string)
}

type healthModel struct {
	handlersList map[string]handlers.HealthHandler
}

func (hm *healthModel) LookupHandler(ctx context.Context, registerMsg healthcontract.ResourceHealthRegistrationMessage) (handlers.HealthHandler, string) {
	logger := radlogger.GetLogger(ctx)
	// For Kubernetes, return Push mode
	if registerMsg.Resource.ResourceKind == resourcekinds.Kubernetes {
		kID := registerMsg.Resource.Identity.Data.(resourcemodel.KubernetesIdentity)
		if hm.handlersList[kID.Kind] == nil {
			// TODO: Convert this log to error once health checks are implemented for all resource kinds
			logger.Info(fmt.Sprintf("ResourceKind: %s-%s does not support health checks. Resource: %+v not monitored by HealthService", registerMsg.Resource.ResourceKind, kID.Kind, registerMsg.Resource.Identity))
			return nil, handlers.HealthHandlerModePush
		}
		return hm.handlersList[kID.Kind], handlers.HealthHandlerModePush
	}

	// For all other resource kinds, the mode is Pull
	if hm.handlersList[registerMsg.Resource.ResourceKind] == nil {
		// TODO: Convert this log to error once health checks are implemented for all resource kinds
		logger.Info(fmt.Sprintf("ResourceKind: %s does not support health checks. Resource: %+v not monitored by HealthService", registerMsg.Resource.ResourceKind, registerMsg.Resource.Identity))
		return nil, handlers.HealthHandlerModePull
	}
	return hm.handlersList[registerMsg.Resource.ResourceKind], handlers.HealthHandlerModePull

}

func NewHealthModel(handlers map[string]handlers.HealthHandler) HealthModel {
	return &healthModel{
		handlersList: handlers,
	}
}
