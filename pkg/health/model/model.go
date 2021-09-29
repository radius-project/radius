// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"github.com/Azure/radius/pkg/health/handlers"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/resourcemodel"
)

type HealthModel interface {
	LookupHandler(registerMsg healthcontract.ResourceHealthRegistrationMessage) (handlers.HealthHandler, string)
}

type healthModel struct {
	handlersList map[string]handlers.HealthHandler
}

func (hm *healthModel) LookupHandler(registerMsg healthcontract.ResourceHealthRegistrationMessage) (handlers.HealthHandler, string) {
	// For Kubernetes, return Push mode
	if registerMsg.Resource.ResourceKind == resourcekinds.Kubernetes {
		kID := registerMsg.Resource.Identity.Data.(resourcemodel.KubernetesIdentity)
		return hm.handlersList[kID.Kind], handlers.HealthHandlerModePush
	}

	// For all other resource kinds, the mode is Pull
	return hm.handlersList[registerMsg.Resource.ResourceKind], handlers.HealthHandlerModePull

}

func NewHealthModel(handlers map[string]handlers.HealthHandler) HealthModel {
	return &healthModel{
		handlersList: handlers,
	}
}
