// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	ucpresources "github.com/project-radius/radius/pkg/ucp/resources"
)

//go:generate mockgen -destination=./mock_recipe_handler.go -package=handlers -self_package github.com/project-radius/radius/pkg/connectorrp/handlers github.com/project-radius/radius/pkg/connectorrp/handlers RecipeHandler
type RecipeHandler interface {
	DeployRecipe(ctx context.Context, templatePath string, subscriptiionID string, resourceGroupName string) ([]string, error)
	Delete(ctx context.Context, id string, apiVersion string) error
}

func NewRecipeHandler(arm *armauth.ArmConfig) RecipeHandler {
	return &azureRecipeHandler{
		arm: arm,
	}
}

type azureRecipeHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureRecipeHandler) DeployRecipe(ctx context.Context, templatePath string, subscriptiionID string, resourceGroupName string) ([]string, error) {
	return []string{}, nil
}
func (handler *azureRecipeHandler) Delete(ctx context.Context, id string, apiVersion string) error {
	parsed, err := ucpresources.Parse(id)
	if err != nil {
		return err
	}

	rc := clients.NewGenericResourceClient(parsed.FindScope(ucpresources.SubscriptionsSegment), handler.arm.Auth)
	_, err = rc.DeleteByID(ctx, id, apiVersion)
	if err != nil {
		if !clients.Is404Error(err) {
			return fmt.Errorf("failed to delete resource %q: %w", id, err)
		}
	}
	return nil
}
