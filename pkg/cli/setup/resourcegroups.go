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

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
)

type ErrUCPResourceGroupCreationFailed struct {
	resp *http.Response
	err  error
}

func (e *ErrUCPResourceGroupCreationFailed) Error() string {
	if e.resp == nil {
		return fmt.Sprintf("failed to create UCP resourceGroup: %s", e.err)
	}

	return fmt.Sprintf("request to create UCP resourceGroup failed with status: %d, response: %+v", e.resp.StatusCode, e.resp)
}

func (e *ErrUCPResourceGroupCreationFailed) Is(target error) bool {
	_, ok := target.(*ErrUCPResourceGroupCreationFailed)
	return ok
}

// TODO remove this when envInit is removed. DO NOT add new uses of this function. Use the generated client.
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

	baseUrl, rt, err := kubernetes.GetBaseUrlAndRoundTripper(kc.Overrides.UCP, kubernetes.UCPType, kc.Context)
	if err != nil {
		return "", &cli.ClusterUnreachableError{Err: err}
	}

	createRgRequest, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s%s/resourceGroups/%s?api-version=%s", baseUrl, plane, resourceGroupName, v20220901privatepreview.Version),
		strings.NewReader(`{
			"location": "global"
		}`))

	if err != nil {
		return "", &ErrUCPResourceGroupCreationFailed{nil, err}
	}
	createRgRequest = createRgRequest.WithContext(ctx)

	resp, err := rt.RoundTrip(createRgRequest)
	if err != nil || (resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK) {
		return "", &ErrUCPResourceGroupCreationFailed{resp, err}
	}

	defer resp.Body.Close()
	var jsonBody map[string]any
	if json.NewDecoder(resp.Body).Decode(&jsonBody) != nil {
		return "", nil
	}

	return jsonBody["id"].(string), nil
}
