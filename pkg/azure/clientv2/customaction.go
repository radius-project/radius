// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// ClientCustomActionResponse is the response we get from invoking a custom action.
type ClientCustomActionResponse struct {
	// Body is the Custom Action response body.
	Body map[string]any
}

// CustomActionClient is the client to invoke custom actions on Azure resources.
// Ex: listSecrets on a MongoDatabase.
type CustomActionClient struct {
	client   *armresources.Client
	pipeline *runtime.Pipeline
	baseURI  string
}

// InvokeCustomAction invokes a custom action on the given resource.
func (client *CustomActionClient) InvokeCustomAction(ctx context.Context, resourceID, apiVersion, action string) (*ClientCustomActionResponse, error) {
	req, err := client.customActionCreateRequest(ctx, resourceID, apiVersion, action)
	if err != nil {
		return nil, err
	}

	resp, err := client.pipeline.Do(req)
	if err != nil {
		return nil, err
	}

	if !runtime.HasStatusCode(resp, http.StatusOK, http.StatusAccepted, http.StatusNoContent) {
		return nil, runtime.NewResponseError(resp)
	}

	body := map[string]any{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	if err != nil {
		return nil, err
	}

	return &ClientCustomActionResponse{
		Body: body,
	}, nil
}

func (client *CustomActionClient) customActionCreateRequest(ctx context.Context, resourceID, apiVersion, action string) (*policy.Request, error) {
	_, err := resources.ParseResource(resourceID)
	if err != nil {
		return nil, err
	}

	if resourceID == "" {
		return nil, errors.New("resourceID cannot be empty")
	}

	if action == "" {
		return nil, errors.New("action cannot be empty")
	}

	urlPath := runtime.JoinPaths(client.baseURI, url.PathEscape(resourceID), url.PathEscape(action))
	req, err := runtime.NewRequest(ctx, http.MethodPost, urlPath)
	if err != nil {
		return nil, err
	}
	reqQP := req.Raw().URL.Query()
	reqQP.Set("api-version", apiVersion)
	req.Raw().URL.RawQuery = reqQP.Encode()
	req.Raw().Header["Accept"] = []string{"application/json"}
	return req, runtime.MarshalAsJSON(req, nil)
}
