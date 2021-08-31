// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"github.com/Azure/radius/pkg/health/handlers"
	"github.com/Azure/radius/pkg/healthcontract"
)

type HealthModel interface {
	LookupHandler(registerMsg healthcontract.ResourceHealthRegistrationMessage) (handlers.HealthHandler, string)
}

type healthModel struct {
	handlersList map[string]handlers.HealthHandler
}

func (hm *healthModel) LookupHandler(registerMsg healthcontract.ResourceHealthRegistrationMessage) (handlers.HealthHandler, string) {
	// For Kubernetes, return Push/Pull model based on the Kubernetes metadata type
	if registerMsg.ResourceInfo.ResourceKind == ResourceKindKubernetes {
		kID, err := healthcontract.ParseK8sResourceID(registerMsg.ResourceInfo.ResourceID)
		if err != nil {
			return nil, ""
		}

		if kID.Kind == healthcontract.KubernetesKindDeployment {
			return hm.handlersList[kID.Kind], handlers.HealthHandlerModePush
		} else if kID.Kind == healthcontract.KubernetesKindService {
			return hm.handlersList[kID.Kind], handlers.HealthHandlerModePull
		}
	}

	// For all other resource kinds, the mode is Pull
	return hm.handlersList[registerMsg.ResourceInfo.ResourceKind], handlers.HealthHandlerModePull

}

func NewHealthModel(handlers map[string]handlers.HealthHandler) HealthModel {
	return &healthModel{
		handlersList: handlers,
	}
}
