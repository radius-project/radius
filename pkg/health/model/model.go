// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"github.com/Azure/radius/pkg/health/handleroptions"
	"github.com/Azure/radius/pkg/health/handlers"
	"github.com/Azure/radius/pkg/health/resourcekinds"
	"github.com/Azure/radius/pkg/healthcontract"
)

type HealthModel interface {
	LookupHandler(registerMsg healthcontract.ResourceHealthRegistrationMessage) (handlers.HealthHandler, string)
}

type healthModel struct {
	handlersList map[string]handlers.HealthHandler
}

func (hm *healthModel) LookupHandler(registerMsg healthcontract.ResourceHealthRegistrationMessage) (handlers.HealthHandler, string) {
	// For Kubernetes, return Push mode
	if registerMsg.ResourceInfo.ResourceKind == resourcekinds.ResourceKindKubernetes {
		kID, err := healthcontract.ParseK8sResourceID(registerMsg.ResourceInfo.ResourceID)
		if err != nil {
			return nil, ""
		}
		return hm.handlersList[kID.Kind], handleroptions.HealthHandlerModePush
	}

	// For all other resource kinds, the mode is Pull
	return hm.handlersList[registerMsg.ResourceInfo.ResourceKind], handleroptions.HealthHandlerModePull

}

func NewHealthModel(handlers map[string]handlers.HealthHandler) HealthModel {
	return &healthModel{
		handlersList: handlers,
	}
}
