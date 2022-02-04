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

func NewGenericHandler(arm armauth.ArmConfig) ResourceHandler {
	return &genericHandler{
		arm: arm,
	}
}

type genericHandler struct {
	arm armauth.ArmConfig
}

func (handler *genericHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)
	return properties, nil
}

func (handler *genericHandler) Delete(ctx context.Context, options DeleteOptions) error {
	return nil
}

func NewGenericHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &genericHealthHandler{
		arm: arm,
	}
}

type genericHealthHandler struct {
	arm armauth.ArmConfig
}

func (handler *genericHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
