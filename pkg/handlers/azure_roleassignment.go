// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/healthcontract"
)

const (
	RoleNameKey = "rolename"
)

// NewAzureRoleAssignmentHandler initializes a new handler for resources of kind RoleAssignment
func NewAzureRoleAssignmentHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureRoleAssignmentHandler{arm: arm}
}

type azureRoleAssignmentHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureRoleAssignmentHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {

	// if !options.Resource.Deployed {
	// 	// TODO: right now this resource is already deployed during the rendering process :(
	// 	// this should be done here instead when we have built a more mature system.
	// }

	return map[string]string{}, nil
}

func (handler *azureRoleAssignmentHandler) Delete(ctx context.Context, options DeleteOptions) error {
	// TODO: right now this resource is deleted in a different handler :(
	// this should be done here instead when we have built a more mature system.

	return nil
}

func NewAzureRoleAssignmentHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &azureRoleAssignmentHealthHandler{arm: arm}
}

type azureRoleAssignmentHealthHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureRoleAssignmentHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
