// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/workspaces"
)

func CreateWorkspaceResourceGroup(ctx context.Context, connection workspaces.Connection, name string) (string, error) {
	id, err := createUCPResourceGroup(ctx, connection, name, "/planes/radius/local")
	if err != nil {
		return "", err
	}

	// TODO: we TEMPORARILY create a resource group in the deployments plane because the deployments RP requires it.
	// We'll remove this in the future.
	_, err = createUCPResourceGroup(ctx, connection, name, "/planes/deployments/local")
	if err != nil {
		return "", err
	}

	return id, nil
}

func createUCPResourceGroup(ctx context.Context, connection workspaces.Connection, resourceGroupName string, plane string) (string, error) {
	kc, ok := connection.(*workspaces.KubernetesConnection)
	if !ok {
		return "", errors.New("only kubernetes connections are supported right now")
	}

	baseUrl, rt, err := kubernetes.GetBaseUrlAndRoundTripper("", kubernetes.UCPType, kc.Context)
	if err != nil {
		return "", err
	}

	createRgRequest, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s%s/resourceGroups/%s", baseUrl, plane, resourceGroupName),
		strings.NewReader(`{}`))
	if err != nil {
		return "", fmt.Errorf("failed to create UCP resourceGroup: %w", err)
	}
	createRgRequest = createRgRequest.WithContext(ctx)

	resp, err := rt.RoundTrip(createRgRequest)
	if err != nil {
		return "", fmt.Errorf("failed to create UCP resourceGroup: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request to create UCP resouceGroup failed with status: %d, request: %+v", resp.StatusCode, resp)
	}
	defer resp.Body.Close()
	var jsonBody map[string]interface{}
	if json.NewDecoder(resp.Body).Decode(&jsonBody) != nil {
		return "", nil
	}

	return jsonBody["id"].(string), nil
}
