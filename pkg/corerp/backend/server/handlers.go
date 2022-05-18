// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"sync"

	asyncctrl "github.com/project-radius/radius/pkg/corerp/backend/controller"
)

type HandlerRegistry struct {
	ctrlMap   map[string]asyncctrl.AsyncControllerInterface
	ctrlMapMu sync.Mutex
}

func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		ctrlMap: map[string]asyncctrl.AsyncControllerInterface{},
	}
}

func (h *HandlerRegistry) RegisterController(name string, ctrl asyncctrl.AsyncControllerInterface) {
	h.ctrlMapMu.Lock()
	defer h.ctrlMapMu.Unlock()

	h.ctrlMap[name] = ctrl
}

func (h *HandlerRegistry) GetController(name string) asyncctrl.AsyncControllerInterface {
	h.ctrlMapMu.Lock()
	defer h.ctrlMapMu.Unlock()

	if h, ok := h.ctrlMap[name]; ok {
		return h
	}

	return nil
}
