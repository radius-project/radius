/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
