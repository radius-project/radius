// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlerv3

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/armerrors"
	"github.com/Azure/radius/pkg/radrp/frontend/resourceproviderv3"
	"github.com/Azure/radius/pkg/radrp/frontend/server"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

// These tests cover the mechanics of how the handler turns HTTP requests into
// the resource IDs and strongly typed data model of the RP.
//
// There's basically no business logic in the handler, all of that is delegated to mocks.

const baseURI = "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application"

type test struct {
	t       *testing.T
	ctrl    *gomock.Controller
	server  *httptest.Server
	handler http.Handler
	rp      *resourceproviderv3.MockResourceProvider
}

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func start(t *testing.T) *test {
	ctrl := gomock.NewController(t)
	rp := resourceproviderv3.NewMockResourceProvider(ctrl)

	options := server.ServerOptions{
		Address:      httptest.DefaultRemoteAddr,
		Authenticate: false,
		Configure: func(router *mux.Router) {
			AddRoutes(rp, router)
		},
	}

	s := server.NewServer(createContext(t), options)
	server := httptest.NewServer(s.Handler)
	h := injectLogger(t, server.Config.Handler)
	t.Cleanup(server.Close)

	return &test{
		t:       t,
		rp:      rp,
		ctrl:    ctrl,
		server:  server,
		handler: h,
	}
}

func injectLogger(t *testing.T, h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		logger, err := radlogger.NewTestLogger(t)
		if err != nil {
			t.Error(err)
			h.ServeHTTP(w, r.WithContext(context.Background()))
			return
		}
		ctx := logr.NewContext(context.Background(), logger)
		h.ServeHTTP(w, r.WithContext(ctx))
	}

	return http.HandlerFunc(fn)
}

func requireJSON(t *testing.T, expected interface{}, w *httptest.ResponseRecorder) {
	bytes, err := json.Marshal(expected)
	require.NoError(t, err)
	require.JSONEq(t, string(bytes), w.Body.String())
}

type FaultingResponse struct {
}

func (r *FaultingResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	return fmt.Errorf("write failure!")
}

func Test_Handler(t *testing.T) {
	testcases := []struct {
		Method      string
		Description string
		URI         string
		Expect      func(*resourceproviderv3.MockResourceProvider) *gomock.Call
		Body        interface{}
	}{
		{
			Method:      "GET",
			Description: "ListApplications",
			URI:         baseURI,
			Expect: func(mock *resourceproviderv3.MockResourceProvider) *gomock.Call {
				return mock.EXPECT().ListApplications(gomock.Any(), gomock.Any())
			},
		},
		{
			Method:      "GET",
			Description: "GetApplication",
			URI:         baseURI + "/test-application",
			Expect: func(mock *resourceproviderv3.MockResourceProvider) *gomock.Call {
				return mock.EXPECT().GetApplication(gomock.Any(), gomock.Any())
			},
		},
		{
			Method:      "PUT",
			Description: "UpdateApplication",
			URI:         baseURI + "/test-application",
			Expect: func(mock *resourceproviderv3.MockResourceProvider) *gomock.Call {
				return mock.EXPECT().UpdateApplication(gomock.Any(), gomock.Any(), gomock.Any())
			},

			// We don't need to include any significant data in the body, we're using mocks for testing
			// here. We just want to cover the code path.
			Body: map[string]interface{}{
				"tags": map[string]interface{}{
					"test-tag": "test-value",
				},
			},
		},
		{
			Method:      "DELETE",
			Description: "DeleteApplication",
			URI:         baseURI + "/test-application",
			Expect: func(mock *resourceproviderv3.MockResourceProvider) *gomock.Call {
				return mock.EXPECT().DeleteApplication(gomock.Any(), gomock.Any())
			},
		},
		{
			Method:      "GET",
			Description: "ListResources",
			URI:         baseURI + "/test-application/test-resource-type",
			Expect: func(mock *resourceproviderv3.MockResourceProvider) *gomock.Call {
				return mock.EXPECT().ListResources(gomock.Any(), gomock.Any())
			},
		},
		{
			Method:      "GET",
			Description: "GetResource",
			URI:         baseURI + "/test-application/test-resource-type/test-resource",
			Expect: func(mock *resourceproviderv3.MockResourceProvider) *gomock.Call {
				return mock.EXPECT().GetResource(gomock.Any(), gomock.Any())
			},
		},
		{
			Method:      "PUT",
			Description: "UpdateResource",
			URI:         baseURI + "/test-application/test-resource-type/test-resource",
			Expect: func(mock *resourceproviderv3.MockResourceProvider) *gomock.Call {
				return mock.EXPECT().UpdateResource(gomock.Any(), gomock.Any(), gomock.Any())
			},

			// We don't need to include any significant data in the body, we're using mocks for testing
			// here. We just want to cover the code path.
			Body: map[string]interface{}{
				"tags": map[string]interface{}{
					"test-tag": "test-value",
				},
			},
		},
		{
			Method:      "DELETE",
			Description: "DeleteResource",
			URI:         baseURI + "/test-application/test-resource-type/test-resource",
			Expect: func(mock *resourceproviderv3.MockResourceProvider) *gomock.Call {
				return mock.EXPECT().DeleteResource(gomock.Any(), gomock.Any())
			},
		},
		{
			Method:      "GET",
			Description: "GetOperation",
			URI:         baseURI + "/test-application/test-resource-type/test-resource/OperationResults/test-operation",
			Expect: func(mock *resourceproviderv3.MockResourceProvider) *gomock.Call {
				return mock.EXPECT().GetOperation(gomock.Any(), gomock.Any())
			},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.Description, func(t *testing.T) {
			t.Run("Success", func(t *testing.T) {
				test := start(t)
				if testcase.Method == "PUT" {
					testcase.Expect(test.rp).Times(1).DoAndReturn(func(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error) {
						return rest.NewOKResponse(map[string]interface{}{}), nil // Empty JSON
					})
				} else {
					testcase.Expect(test.rp).Times(1).DoAndReturn(func(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
						return rest.NewOKResponse(map[string]interface{}{}), nil // Empty JSON
					})
				}

				var err error
				body := []byte{}
				if testcase.Body != nil {
					body, err = json.Marshal(testcase.Body)
					require.NoError(t, err)
				}

				req := httptest.NewRequest(testcase.Method, testcase.URI, bytes.NewBuffer(body))
				w := httptest.NewRecorder()

				test.handler.ServeHTTP(w, req)

				require.Equal(t, 200, w.Code)
				requireJSON(t, map[string]interface{}{}, w)
			})

			t.Run("Error", func(t *testing.T) {
				test := start(t)
				if testcase.Method == "PUT" {
					testcase.Expect(test.rp).Times(1).DoAndReturn(func(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error) {
						return nil, fmt.Errorf("error!") // Empty JSON
					})
				} else {
					testcase.Expect(test.rp).Times(1).DoAndReturn(func(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
						return nil, fmt.Errorf("error!") // Empty JSON
					})
				}

				var err error
				body := []byte{}
				if testcase.Body != nil {
					body, err = json.Marshal(testcase.Body)
					require.NoError(t, err)
				}

				req := httptest.NewRequest(testcase.Method, testcase.URI, bytes.NewBuffer(body))
				w := httptest.NewRecorder()

				test.handler.ServeHTTP(w, req)

				require.Equal(t, 500, w.Code)
				requireJSON(t, &armerrors.ErrorResponse{
					Error: armerrors.ErrorDetails{
						Message: "error!",
					},
				}, w)
			})

			t.Run("Write-Failure", func(t *testing.T) {
				test := start(t)
				if testcase.Method == "PUT" {
					testcase.Expect(test.rp).Times(1).DoAndReturn(func(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error) {
						return &FaultingResponse{}, nil
					})
				} else {
					testcase.Expect(test.rp).Times(1).DoAndReturn(func(ctx context.Context, id azresources.ResourceID) (rest.Response, error) {
						return &FaultingResponse{}, nil
					})
				}

				var err error
				body := []byte{}
				if testcase.Body != nil {
					body, err = json.Marshal(testcase.Body)
					require.NoError(t, err)
				}

				req := httptest.NewRequest(testcase.Method, testcase.URI, bytes.NewBuffer(body))
				w := httptest.NewRecorder()

				test.handler.ServeHTTP(w, req)

				require.Equal(t, 500, w.Code)
				requireJSON(t, &armerrors.ErrorResponse{
					Error: armerrors.ErrorDetails{
						Message: "write failure!",
					},
				}, w)
			})

			// Remaining sub-tests are for PUT methods - they deal with the request body.
			if testcase.Method != "PUT" {
				return
			}

			t.Run("Invalid-JSON", func(t *testing.T) {
				test := start(t)
				testcase.Expect(test.rp).Times(1).DoAndReturn(func(ctx context.Context, id azresources.ResourceID, body []byte) (rest.Response, error) {
					return &FaultingResponse{}, nil
				})

				req := httptest.NewRequest(testcase.Method, testcase.URI, bytes.NewBuffer([]byte("not json")))
				w := httptest.NewRecorder()

				test.handler.ServeHTTP(w, req)

				require.Equal(t, 500, w.Code)
				requireJSON(t, &armerrors.ErrorResponse{
					Error: armerrors.ErrorDetails{
						Message: "write failure!",
					},
				}, w)
			})

			t.Run("Validation-Failure", func(t *testing.T) {
				// Placeholder for adding schema validation support
			})
		})
	}
}
