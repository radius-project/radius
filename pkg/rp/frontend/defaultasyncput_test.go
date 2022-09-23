// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package frontend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

const testHeaderfile = "requestheaders20220315privatepreview.json"

func getTestModels20220315privatepreview() (*v20220315privatepreview.HTTPRouteResource, *datamodel.HTTPRoute, *v20220315privatepreview.HTTPRouteResource) {
	rawInput := radiustesting.ReadFixture("httproute20220315privatepreview_input.json")
	hrtInput := &v20220315privatepreview.HTTPRouteResource{}
	_ = json.Unmarshal(rawInput, hrtInput)

	rawDataModel := radiustesting.ReadFixture("httproute20220315privatepreview_datamodel.json")
	hrtDataModel := &datamodel.HTTPRoute{}
	_ = json.Unmarshal(rawDataModel, hrtDataModel)

	rawExpectedOutput := radiustesting.ReadFixture("httproute20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.HTTPRouteResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return hrtInput, hrtDataModel, expectedOutput
}

func setupTest(tb testing.TB) (func(testing.TB), *store.MockStorageClient, *statusmanager.MockStatusManager) {
	mctrl := gomock.NewController(tb)
	mds := store.NewMockStorageClient(mctrl)
	msm := statusmanager.NewMockStatusManager(mctrl)

	return func(tb testing.TB) {
		mctrl.Finish()
	}, mds, msm
}

func TestDefaultAsyncPut_Create(t *testing.T) {
	createCases := []struct {
		desc    string
		getErr  error
		saveErr error
		qErr    error
		rbErr   error
		rCode   int
		rErr    error
	}{
		{
			"async-create-new-httproute-success",
			&store.ErrNotFound{},
			nil,
			nil,
			nil,
			http.StatusCreated,
			nil,
		},
		{
			"async-create-new-httproute-concurrency-error",
			&store.ErrConcurrency{},
			nil,
			nil,
			nil,
			http.StatusCreated,
			&store.ErrConcurrency{},
		},
		{
			"async-create-new-httproute-enqueue-error",
			&store.ErrNotFound{},
			nil,
			errors.New("enqueuer client is unset"),
			nil,
			http.StatusInternalServerError,
			errors.New("enqueuer client is unset"),
		},
	}

	for _, tt := range createCases {
		t.Run(tt.desc, func(t *testing.T) {
			teardownTest, mds, msm := setupTest(t)
			defer teardownTest(t)

			httprouteInput, httprouteDataModel, _ := getTestModels20220315privatepreview()

			w := httptest.NewRecorder()
			req, err := radiustesting.GetARMTestHTTPRequest(context.Background(), http.MethodPut, testHeaderfile, httprouteInput)
			require.NoError(t, err)

			ctx := radiustesting.ARMTestContextFromRequest(req)
			sCtx := v1.ARMRequestContextFromContext(ctx)

			mds.EXPECT().Get(gomock.Any(), gomock.Any()).
				Return(&store.Object{}, tt.getErr).
				Times(1)

			if tt.getErr == nil || errors.Is(&store.ErrNotFound{}, tt.getErr) {
				mds.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(tt.saveErr).
					Times(1)

				if tt.saveErr == nil {
					msm.EXPECT().QueueAsyncOperation(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(tt.qErr).
						Times(1)

					if tt.qErr != nil {
						mds.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).
							Return(tt.rbErr).
							Times(1)
					}
				}
			}

			opts := ctrl.Options{
				StorageClient: mds,
				StatusManager: msm,
			}

			ctl, err := NewDefaultAsyncPut(opts, converter.HTTPRouteDataModelFromVersioned, converter.HTTPRouteDataModelToVersioned)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, req)

			if tt.rErr != nil {
				require.Error(t, tt.rErr)
			} else {
				require.NoError(t, err)

				_ = resp.Apply(ctx, w, req)
				require.Equal(t, tt.rCode, w.Result().StatusCode)

				locationHeader := getAsyncLocationPath(sCtx, httprouteDataModel.TrackedResource.Location, "operationResults", req)
				require.NotNil(t, w.Header().Get("Location"))
				require.Equal(t, locationHeader, w.Header().Get("Location"))

				azureAsyncOpHeader := getAsyncLocationPath(sCtx, httprouteDataModel.TrackedResource.Location, "operationStatuses", req)
				require.NotNil(t, w.Header().Get("Azure-AsyncOperation"))
				require.Equal(t, azureAsyncOpHeader, w.Header().Get("Azure-AsyncOperation"))
			}
		})
	}
}

func TestDefaultAsyncPut_Update(t *testing.T) {
	updateCases := []struct {
		desc               string
		curState           v1.ProvisioningState
		versionedInputFile string
		datamodelFile      string
		getErr             error
		skipSave           bool
		saveErr            error
		qErr               error
		rbErr              error
		rCode              int
		rErr               error
	}{
		{
			"async-update-existing-httproute-success",
			v1.ProvisioningStateSucceeded,
			"httproute20220315privatepreview_input.json",
			"httproute20220315privatepreview_datamodel.json",
			nil,
			false,
			nil,
			nil,
			nil,
			http.StatusAccepted,
			nil,
		},
		{
			"async-update-existing-httproute-mismatched-appid",
			v1.ProvisioningStateSucceeded,
			"httproute20220315privatepreview_input_appid.json",
			"httproute20220315privatepreview_datamodel.json",
			nil,
			true,
			nil,
			nil,
			nil,
			http.StatusBadRequest,
			nil,
		},
		{
			"async-update-existing-httproute-concurrency-error",
			v1.ProvisioningStateSucceeded,
			"httproute20220315privatepreview_input.json",
			"httproute20220315privatepreview_datamodel.json",
			nil,
			false,
			&store.ErrConcurrency{},
			nil,
			nil,
			http.StatusInternalServerError,
			&store.ErrConcurrency{},
		},
		{
			"async-update-existing-httproute-save-error",
			v1.ProvisioningStateSucceeded,
			"httproute20220315privatepreview_input.json",
			"httproute20220315privatepreview_datamodel.json",
			nil,
			false,
			&store.ErrInvalid{Message: "testing initial save err"},
			nil,
			nil,
			http.StatusInternalServerError,
			&store.ErrInvalid{Message: "testing initial save err"},
		},
		{
			"async-update-existing-httproute-enqueue-error",
			v1.ProvisioningStateSucceeded,
			"httproute20220315privatepreview_input.json",
			"httproute20220315privatepreview_datamodel.json",
			nil,
			false,
			nil,
			&store.ErrInvalid{Message: "testing initial save err"},
			nil,
			http.StatusInternalServerError,
			&store.ErrInvalid{Message: "testing initial save err"},
		},
	}

	for _, tt := range updateCases {
		t.Run(tt.desc, func(t *testing.T) {
			teardownTest, mds, msm := setupTest(t)
			defer teardownTest(t)

			httprouteInput := &v20220315privatepreview.HTTPRouteResource{}
			_ = json.Unmarshal(radiustesting.ReadFixture(tt.versionedInputFile), httprouteInput)

			httprouteDataModel := &datamodel.HTTPRoute{}
			_ = json.Unmarshal(radiustesting.ReadFixture(tt.datamodelFile), httprouteDataModel)

			httprouteDataModel.InternalMetadata.AsyncProvisioningState = tt.curState

			w := httptest.NewRecorder()
			req, err := radiustesting.GetARMTestHTTPRequest(context.Background(), http.MethodPatch, testHeaderfile, httprouteInput)
			require.NoError(t, err)

			ctx := radiustesting.ARMTestContextFromRequest(req)
			sCtx := v1.ARMRequestContextFromContext(ctx)

			so := &store.Object{
				Metadata: store.Metadata{ID: sCtx.ResourceID.String()},
				Data:     httprouteDataModel,
			}

			mds.EXPECT().Get(gomock.Any(), gomock.Any()).
				Return(so, tt.getErr).
				Times(1)

			if tt.getErr == nil && !tt.skipSave {
				mds.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(tt.saveErr).
					Times(1)

				if tt.saveErr == nil {
					msm.EXPECT().QueueAsyncOperation(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(tt.qErr).
						Times(1)

					if tt.qErr != nil {
						mds.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).
							Return(tt.rbErr).
							Times(1)
					}
				}
			}

			opts := ctrl.Options{
				StorageClient: mds,
				StatusManager: msm,
			}

			ctl, err := NewDefaultAsyncPut(opts, converter.HTTPRouteDataModelFromVersioned, converter.HTTPRouteDataModelToVersioned)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, req)
			if resp != nil {
				_ = resp.Apply(ctx, w, req)
				require.Equal(t, tt.rCode, w.Result().StatusCode)
			}

			if tt.rCode == http.StatusAccepted {
				require.NoError(t, err)

				locationHeader := getAsyncLocationPath(sCtx, httprouteDataModel.TrackedResource.Location, "operationResults", req)
				require.NotNil(t, w.Header().Get("Location"))
				require.Equal(t, locationHeader, w.Header().Get("Location"))

				azureAsyncOpHeader := getAsyncLocationPath(sCtx, httprouteDataModel.TrackedResource.Location, "operationStatuses", req)
				require.NotNil(t, w.Header().Get("Azure-AsyncOperation"))
				require.Equal(t, azureAsyncOpHeader, w.Header().Get("Azure-AsyncOperation"))
			}

			if tt.rErr != nil {
				require.ErrorIs(t, tt.rErr, err)
			}
		})
	}
}

func getAsyncLocationPath(sCtx *v1.ARMRequestContext, location string, resourceType string, req *http.Request) string {
	dest := url.URL{
		Host:   req.Host,
		Scheme: req.URL.Scheme,
		Path: fmt.Sprintf("%s/providers/%s/locations/%s/%s/%s", sCtx.ResourceID.PlaneScope(),
			sCtx.ResourceID.ProviderNamespace(), location, resourceType, sCtx.OperationID.String()),
	}

	query := url.Values{}
	query.Add("api-version", sCtx.APIVersion)
	dest.RawQuery = query.Encode()

	// In production this is the header we get from app service for the 'real' protocol
	protocol := req.Header.Get("X-Forwarded-Proto")
	if protocol != "" {
		dest.Scheme = protocol
	}

	if dest.Scheme == "" {
		dest.Scheme = "http"
	}

	return dest.String()
}
