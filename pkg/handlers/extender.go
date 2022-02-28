// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/healthcontract"
)

func NewExtenderHandler(arm armauth.ArmConfig) ResourceHandler {
	return &extenderHandler{
		arm: arm,
	}
}

type extenderHandler struct {
	arm armauth.ArmConfig
}

func (handler *extenderHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)
	return properties, nil
}

func (handler *extenderHandler) Delete(ctx context.Context, options DeleteOptions) error {
	return nil
}

func NewExtenderHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &extenderHealthHandler{
		arm: arm,
	}
}

type extenderHealthHandler struct {
	arm armauth.ArmConfig
}

func (handler *extenderHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
