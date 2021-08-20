// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"github.com/Azure/radius/pkg/health/handlers"
)

type HealthModel interface {
	LookupHandler(resourceType string) handlers.HealthHandler
}

type healthModel struct {
	handlersList map[string]handlers.HealthHandler
}

func (hm *healthModel) LookupHandler(resourceType string) handlers.HealthHandler {
	return hm.handlersList[resourceType]
}

func NewModel(handlers map[string]handlers.HealthHandler) HealthModel {
	return &healthModel{
		handlersList: handlers,
	}
}
