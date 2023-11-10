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

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

func Test_NewLinkedResourceUpdateErrorResponse(t *testing.T) {
	errTests := []struct {
		desc     string
		oldAppID string
		oldEnvID string
		newAppID string
		newEnvID string
		msg      string
	}{
		{
			desc:     "application_and_environment_ids",
			oldAppID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/test-application",
			oldEnvID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment",
			newAppID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/updated-application",
			newEnvID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment",
			msg:      "Attempted to deploy existing resource 'test-container-0' which has a different application and/or environment. Options to resolve the conflict are: change the name of the 'test-container-0' resource in 'updated-application' application and 'test-environment' environment to create a new resource, or use '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/test-application' application and '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment' environment to update the existing resource 'test-container-0'.",
		}, {
			desc:     "only_application_id_in_existing_resource",
			oldAppID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/test-application",
			oldEnvID: "",
			newAppID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/updated-application",
			newEnvID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment",
			msg:      "Attempted to deploy existing resource 'test-container-0' which has a different application and/or environment. Options to resolve the conflict are: change the name of the 'test-container-0' resource in 'updated-application' application and 'test-environment' environment to create a new resource, or use '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/test-application' application and '' environment to update the existing resource 'test-container-0'.",
		}, {
			desc:     "only_application_id",
			oldAppID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/test-application",
			oldEnvID: "",
			newAppID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/updated-application",
			newEnvID: "",
			msg:      "Attempted to deploy existing resource 'test-container-0' which has a different application and/or environment. Options to resolve the conflict are: change the name of the 'test-container-0' resource in 'updated-application' application to create a new resource, or use '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/test-application' application and '' environment to update the existing resource 'test-container-0'.",
		}, {
			desc:     "only_environment",
			oldAppID: "",
			oldEnvID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment",
			newAppID: "",
			newEnvID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/updated-environment",
			msg:      "Attempted to deploy existing resource 'test-container-0' which has a different application and/or environment. Options to resolve the conflict are: change the name of the 'test-container-0' resource in 'updated-environment' environment to create a new resource, or use '' application and '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment' environment to update the existing resource 'test-container-0'.",
		}, {
			desc:     "invalid_id",
			oldAppID: "",
			oldEnvID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment",
			newAppID: "",
			newEnvID: "invalid_id",
			msg:      "Attempted to deploy existing resource 'test-container-0' which has a different application and/or environment. Options to resolve the conflict are: change the name of the 'test-container-0' resource in 'invalid_id' environment to create a new resource, or use '' application and '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environment/test-environment' environment to update the existing resource 'test-container-0'.",
		},
	}

	resource, err := resources.ParseResource("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/containers/test-container-0")
	require.NoError(t, err)

	for _, tt := range errTests {
		t.Run(tt.desc, func(t *testing.T) {
			expctedResp := &BadRequestResponse{
				Body: v1.ErrorResponse{
					Error: v1.ErrorDetails{
						Code:    v1.CodeInvalid,
						Message: tt.msg,
						Target:  resource.String(),
					},
				},
			}
			oldResourceProp := &rpv1.BasicResourceProperties{
				Application: tt.oldAppID,
				Environment: tt.oldEnvID,
			}
			newResourceProp := &rpv1.BasicResourceProperties{
				Application: tt.newAppID,
				Environment: tt.newEnvID,
			}
			resp := NewLinkedResourceUpdateErrorResponse(resource, oldResourceProp, newResourceProp)

			require.Equal(t, expctedResp, resp)
		})
	}
}

func Test_OKResponse_Empty(t *testing.T) {
	response := NewOKResponse(nil)

	req := httptest.NewRequest("GET", "http://example.com", nil)
	w := httptest.NewRecorder()

	err := response.Apply(context.TODO(), w, req)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string(nil), w.Header()["Content-Type"])
	require.Empty(t, w.Body.Bytes())
}

func Test_OKResponse_WithBody(t *testing.T) {
	payload := map[string]string{
		"message": "hi there!",
	}
	response := NewOKResponse(payload)

	req := httptest.NewRequest("GET", "http://example.com", nil)
	w := httptest.NewRecorder()

	err := response.Apply(context.TODO(), w, req)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"application/json"}, w.Header()["Content-Type"])

	body := map[string]string{}
	err = json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	require.Equal(t, payload, body)
}

func TestGetAsyncLocationPath(t *testing.T) {
	operationID := uuid.New()

	testCases := []struct {
		desc       string
		base       string
		rID        string
		loc        string
		opID       uuid.UUID
		av         string
		or         string
		os         string
		referer    url.URL
		retryAfter time.Duration
	}{
		{
			"ucp-test-headers-custom-retry-after",
			"https://ucp.dev",
			"/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/containers/test-container-0",
			v1.LocationGlobal,
			operationID,
			"2023-10-01-preview",
			fmt.Sprintf("/planes/radius/local/providers/Applications.Core/locations/global/operationResults/%s", operationID.String()),
			fmt.Sprintf("/planes/radius/local/providers/Applications.Core/locations/global/operationStatuses/%s", operationID.String()),
			url.URL{
				Scheme: "https",
				Host:   "ucp.dev",
				Path:   "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/containers/test-container-0",
			},
			10 * time.Second,
		},
		{
			"ucp-test-headers",
			"https://ucp.dev",
			"/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/containers/test-container-0",
			v1.LocationGlobal,
			operationID,
			"2023-10-01-preview",
			fmt.Sprintf("/planes/radius/local/providers/Applications.Core/locations/global/operationResults/%s", operationID.String()),
			fmt.Sprintf("/planes/radius/local/providers/Applications.Core/locations/global/operationStatuses/%s", operationID.String()),
			url.URL{
				Scheme: "https",
				Host:   "ucp.dev",
				Path:   "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/containers/test-container-0",
			},
			time.Second * 0,
		},
		{
			"arm-test-headers",
			"https://azure.dev",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Core/containers/test-container-0",
			v1.LocationGlobal,
			operationID,
			"2023-10-01-preview",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/providers/Applications.Core/locations/global/operationResults/%s", operationID.String()),
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/providers/Applications.Core/locations/global/operationStatuses/%s", operationID.String()),
			url.URL{
				Scheme: "https",
				Host:   "azure.dev",
				Path:   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/test-rg/providers/Applications.Core/containers/test-container-0",
			},
			time.Second * 0,
		},
		{
			"ucp-test-headers",
			"https://ucp.dev",
			"/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/containers/test-container-0",
			v1.LocationGlobal,
			operationID,
			"",
			fmt.Sprintf("/planes/radius/local/providers/Applications.Core/locations/global/operationResults/%s", operationID.String()),
			fmt.Sprintf("/planes/radius/local/providers/Applications.Core/locations/global/operationStatuses/%s", operationID.String()),
			url.URL{
				Scheme: "https",
				Host:   "ucp.dev",
				Path:   "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/containers/test-container-0",
			},
			time.Second * 0,
		},
		{
			"empty-path-headers",
			"https://ucp.dev",
			"/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/containers/test-container-0",
			v1.LocationGlobal,
			operationID,
			"",
			fmt.Sprintf("/planes/radius/local/providers/Applications.Core/locations/global/operationResults/%s", operationID.String()),
			fmt.Sprintf("/planes/radius/local/providers/Applications.Core/locations/global/operationStatuses/%s", operationID.String()),
			url.URL{
				Scheme: "",
				Host:   "",
				Path:   "",
			},
			time.Second * 0,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.desc, func(t *testing.T) {
			resourceID, err := resources.ParseResource(tt.rID)
			require.NoError(t, err)

			body := &datamodel.ContainerResource{}
			r := NewAsyncOperationResponse(body, tt.loc, http.StatusAccepted, resourceID, tt.opID, tt.av, "", "")
			if tt.retryAfter != time.Second*0 {
				r.RetryAfter = tt.retryAfter
			}

			req := httptest.NewRequest("GET", tt.base, nil)
			req.Header.Add(v1.RefererHeader, tt.referer.String())
			w := httptest.NewRecorder()
			err = r.Apply(context.Background(), w, req)
			require.NoError(t, err)

			require.NotNil(t, w.Header().Get("Location"))
			if tt.av == "" {
				require.Equal(t, tt.base+tt.or, w.Header().Get("Location"))
			} else {
				require.Equal(t, tt.base+tt.or+"?api-version="+tt.av, w.Header().Get("Location"))
			}

			require.NotNil(t, w.Header().Get("Azure-AsyncHeader"))
			if tt.av == "" {
				require.Equal(t, tt.base+tt.os, w.Header().Get("Azure-AsyncOperation"))

			} else {
				require.Equal(t, tt.base+tt.os+"?api-version="+tt.av, w.Header().Get("Azure-AsyncOperation"))
			}

			require.NotNil(t, w.Header().Get("Retry-After"))
			if tt.retryAfter == time.Second*0 {
				require.Equal(t, "60", w.Header().Get("Retry-After"))
			} else {
				parsed, err := strconv.ParseInt(w.Header().Get("Retry-After"), 10, 64)
				require.NoError(t, err)

				// Yes, this looks weird but it's the correct way to multiply by a non-constant value.
				duration := time.Duration(parsed) * time.Second
				require.Equal(t, tt.retryAfter, duration)
			}
		})
	}
}
