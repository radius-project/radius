// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/project-radius/radius/pkg/azure/clients"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"
)

// DeployRecipe fetches the recipe ARM JSON template from ACR and deploys it
func (handler *armHandler) DeployRecipe(ctx context.Context, templatePath string, subscriptiionID string, resourceGroupName string) ([]string, error) {
	registryRepo, tag := strings.Split(templatePath, ":")[0], strings.Split(templatePath, ":")[1]
	repo, err := remote.NewRepository(registryRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to create client to ACR %s", err.Error())
	}
	layerDigest, err := fetchLayertDigest(ctx, repo, tag)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recipe manifest from ACR %s", err.Error())
	}
	recipeByte, err := fetchLayertBlob(ctx, repo, layerDigest)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recipe template from ACR %s", err.Error())
	}

	//deploy the ARM JSON template
	outputId, err := handler.deploy(ctx, recipeByte, subscriptiionID, resourceGroupName)
	if err != nil {
		return nil, err
	}
	return outputId, nil
}

func (handler *armHandler) deploy(ctx context.Context, recipeData []byte, subscriptionID, resourceGroupName string) ([]string, error) {
	dClient := clients.NewDeploymentsClient(subscriptionID, handler.arm.Auth)
	deploymtName := "recipe" + time.Now().String()
	contents := make(map[string]interface{})
	err := json.Unmarshal(recipeData, &contents)
	if err != nil {
		return nil, err
	}
	deploymentFuture, err := dClient.CreateOrUpdate(
		ctx,
		resourceGroupName,
		deploymtName,
		resources.Deployment{
			Properties: &resources.DeploymentProperties{
				Template: contents,
				Mode:     resources.DeploymentModeIncremental,
			},
		},
	)
	if err != nil {
		return nil, err
	}
	err = deploymentFuture.WaitForCompletionRef(ctx, dClient.BaseClient.Client)
	if err != nil {
		return nil, err
	}
	resp, err := deploymentFuture.Result(dClient)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, id := range *resp.Properties.OutputResources {
		result = append(result, *id.ID)
	}
	return result, nil
}

func fetchLayertDigest(ctx context.Context, repo *remote.Repository, tag string) (string, error) {
	// resolves a manifest descriptor
	descriptor, err := repo.Resolve(ctx, tag)
	if err != nil {
		return "", err
	}
	rc, err := repo.Fetch(ctx, descriptor)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	manifestBlob, err := content.ReadAll(rc, descriptor)
	if err != nil {
		return "", err
	}
	var manifest map[string]interface{}
	err = json.Unmarshal(manifestBlob, &manifest)
	if err != nil {
		return "", err
	}
	// get the layers digest to fetch the blob
	layer, ok := manifest["layers"].([]interface{})[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("failed to decode the layers from manifest")
	}
	layerDigest, ok := layer["digest"].(string)
	if !ok {
		return "", fmt.Errorf("failed to decode the layers digest from manifest")
	}
	return layerDigest, nil
}

func fetchLayertBlob(ctx context.Context, repo *remote.Repository, layerDigest string) ([]byte, error) {
	// resolves a layer blob descriptor
	descriptor, err := repo.Blobs().Resolve(ctx, layerDigest)
	if err != nil {
		return nil, err
	}
	rc, err := repo.Fetch(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	pulledBlob, err := content.ReadAll(rc, descriptor)
	if err != nil {
		return nil, err
	}
	return pulledBlob, nil
}
