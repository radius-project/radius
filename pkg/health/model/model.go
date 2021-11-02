// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"context"
	"sync"

	"github.com/Azure/radius/pkg/health/handlers"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/resourcemodel"
)

type HealthModel interface {
	LookupHandler(ctx context.Context, registerMsg healthcontract.ResourceHealthRegistrationMessage) (handlers.HealthHandler, string)
	GetWaitGroup() *sync.WaitGroup
}

type healthModel struct {
	handlersList map[string]handlers.HealthHandler
	wg           *sync.WaitGroup
}

func (hm *healthModel) LookupHandler(ctx context.Context, registerMsg healthcontract.ResourceHealthRegistrationMessage) (handlers.HealthHandler, string) {
	// For Kubernetes, return Push mode
	if registerMsg.Resource.ResourceKind == resourcekinds.Kubernetes {
		kID := registerMsg.Resource.Identity.Data.(resourcemodel.KubernetesIdentity)
		if hm.handlersList[kID.Kind] == nil {
			return nil, handlers.HealthHandlerModePush
		}
		return hm.handlersList[kID.Kind], handlers.HealthHandlerModePush
	}

	// For all other resource kinds, the mode is Pull
	if hm.handlersList[registerMsg.Resource.ResourceKind] == nil {
		return nil, handlers.HealthHandlerModePull
	}
	return hm.handlersList[registerMsg.Resource.ResourceKind], handlers.HealthHandlerModePull

}

func (hm *healthModel) GetWaitGroup() *sync.WaitGroup {
	return hm.wg
}

// The health service has multiple goroutines running. The wait group parameter here is used to ensure that all goroutines are stopped
// when an exit signal is received. This parameter could also be used by tests to wait till all goroutines stop and then stop the test.
func NewHealthModel(handlers map[string]handlers.HealthHandler, wg *sync.WaitGroup) HealthModel {
	return &healthModel{
		handlersList: handlers,
		wg:           wg,
	}
}
