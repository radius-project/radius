// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
)

type ErrUCPResourceGroupCreationFailed struct {
	resp *http.Response
	err  error
}

// # Function Explanation
// 
//	ErrUCPResourceGroupCreationFailed is an error handling function that returns a descriptive error message based on the 
//	parameters passed in. If the response parameter is nil, it returns a message with the error passed in, otherwise it 
//	returns a message with the response status code and response object. This is useful for callers of this function to 
//	understand why the UCP resourceGroup creation failed.
func (e *ErrUCPResourceGroupCreationFailed) Error() string {
	if e.resp == nil {
		return fmt.Sprintf("failed to create UCP resourceGroup: %s", e.err)
	}

	return fmt.Sprintf("request to create UCP resourceGroup failed with status: %d, response: %+v", e.resp.StatusCode, e.resp)
}

// # Function Explanation
// 
//	ErrUCPResourceGroupCreationFailed is an error type that allows callers to check if a given error is of this type, which 
//	can be useful for error handling.
func (e *ErrUCPResourceGroupCreationFailed) Is(target error) bool {
	_, ok := target.(*ErrUCPResourceGroupCreationFailed)
	return ok
}

// TODO remove this when envInit is removed. DO NOT add new uses of this function. Use the generated client.
//
// # Function Explanation
// 
//	CreateWorkspaceResourceGroup creates two resource groups in UCP, one in the radius plane and one in the deployments 
//	plane, and returns the ID of the resource group in the radius plane. If an error occurs, it is returned to the caller.
func CreateWorkspaceResourceGroup(ctx context.Context, connection sdk.Connection, name string) (string, error) {
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

func createUCPResourceGroup(ctx context.Context, connection sdk.Connection, resourceGroupName string, plane string) (string, error) {
	createRgRequest, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s%s/resourceGroups/%s?api-version=%s", connection.Endpoint(), plane, resourceGroupName, v20220901privatepreview.Version),
		strings.NewReader(`{
			"location": "global"
		}`))

	if err != nil {
		return "", &ErrUCPResourceGroupCreationFailed{nil, err}
	}
	createRgRequest = createRgRequest.WithContext(ctx)
	createRgRequest.Header.Add("Content-Type", "application/json")

	resp, err := connection.Client().Do(createRgRequest)
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
