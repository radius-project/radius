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

package containers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestCreateOrUpdateContainerRun_20220315PrivatePreview(t *testing.T) {
	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *statusmanager.MockStatusManager) {
		mctrl := gomock.NewController(t)
		mds := store.NewMockStorageClient(mctrl)
		msm := statusmanager.NewMockStatusManager(mctrl)

		return func(tb testing.TB) {
			mctrl.Finish()
		}, mds, msm
	}

	/*
		Creating a container resource in an async way has multiple operations with branching:
		1. Get Resource
		2. [Conditional] If resource exists, check if there is an ongoing operation on it
		3. Save Resource
		4. Queue Resource
		5. [Conditional] If Queue has an error then Rollback changes
		6. [Conditional] Update the record state to Failed
	*/
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
			"async-create-new-container-success",
			&store.ErrNotFound{},
			nil,
			nil,
			nil,
			http.StatusCreated,
			nil,
		},
		{
			"async-create-new-container-concurrency-error",
			&store.ErrConcurrency{},
			nil,
			nil,
			nil,
			http.StatusCreated,
			&store.ErrConcurrency{},
		},
		{
			"async-create-new-container-enqueue-error",
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

			containerInput := &v20220315privatepreview.ContainerResource{}
			_ = json.Unmarshal(testutil.ReadFixture("container20220315privatepreview_input.json"), containerInput)

			containerDataModel := &datamodel.ContainerResource{}
			_ = json.Unmarshal(testutil.ReadFixture("container20220315privatepreview_datamodel.json"), containerDataModel)

			w := httptest.NewRecorder()
			req, err := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodPut, testHeaderfile, containerInput)
			require.NoError(t, err)

			ctx := testutil.ARMTestContextFromRequest(req)
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

			ctl, err := NewCreateOrUpdateContainer(opts)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, w, req)

			if tt.rErr != nil {
				require.Error(t, tt.rErr)
			} else {
				require.NoError(t, err)

				_ = resp.Apply(ctx, w, req)
				require.Equal(t, tt.rCode, w.Result().StatusCode)

				locationHeader := getAsyncLocationPath(sCtx, containerDataModel.TrackedResource.Location, "operationResults", req)
				require.NotNil(t, w.Header().Get("Location"))
				require.Equal(t, locationHeader, w.Header().Get("Location"))

				azureAsyncOpHeader := getAsyncLocationPath(sCtx, containerDataModel.TrackedResource.Location, "operationStatuses", req)
				require.NotNil(t, w.Header().Get("Azure-AsyncOperation"))
				require.Equal(t, azureAsyncOpHeader, w.Header().Get("Azure-AsyncOperation"))
			}
		})
	}

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
			"async-update-existing-container-success",
			v1.ProvisioningStateSucceeded,
			"container20220315privatepreview_input.json",
			"container20220315privatepreview_datamodel.json",
			nil,
			false,
			nil,
			nil,
			nil,
			http.StatusAccepted,
			nil,
		},
		{
			"async-update-existing-container-mismatched-appid",
			v1.ProvisioningStateSucceeded,
			"container20220315privatepreview_input_appid.json",
			"container20220315privatepreview_datamodel.json",
			nil,
			true,
			nil,
			nil,
			nil,
			http.StatusBadRequest,
			nil,
		},
		{
			"async-update-existing-container-concurrency-error",
			v1.ProvisioningStateSucceeded,
			"container20220315privatepreview_input.json",
			"container20220315privatepreview_datamodel.json",
			nil,
			false,
			&store.ErrConcurrency{},
			nil,
			nil,
			http.StatusInternalServerError,
			&store.ErrConcurrency{},
		},
		{
			"async-update-existing-container-save-error",
			v1.ProvisioningStateSucceeded,
			"container20220315privatepreview_input.json",
			"container20220315privatepreview_datamodel.json",
			nil,
			false,
			&store.ErrInvalid{Message: "testing initial save err"},
			nil,
			nil,
			http.StatusInternalServerError,
			&store.ErrInvalid{Message: "testing initial save err"},
		},
		{
			"async-update-existing-container-enqueue-error",
			v1.ProvisioningStateSucceeded,
			"container20220315privatepreview_input.json",
			"container20220315privatepreview_datamodel.json",
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

			containerInput := &v20220315privatepreview.ContainerResource{}
			err := json.Unmarshal(testutil.ReadFixture(tt.versionedInputFile), containerInput)
			require.NoError(t, err)

			containerDataModel := &datamodel.ContainerResource{}
			err = json.Unmarshal(testutil.ReadFixture(tt.datamodelFile), containerDataModel)
			require.NoError(t, err)

			containerDataModel.InternalMetadata.AsyncProvisioningState = tt.curState

			w := httptest.NewRecorder()
			req, err := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodPatch, testHeaderfile, containerInput)
			require.NoError(t, err)

			ctx := testutil.ARMTestContextFromRequest(req)
			sCtx := v1.ARMRequestContextFromContext(ctx)

			so := &store.Object{
				Metadata: store.Metadata{ID: sCtx.ResourceID.String()},
				Data:     containerDataModel,
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

			ctl, err := NewCreateOrUpdateContainer(opts)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, w, req)

			if resp != nil {
				_ = resp.Apply(ctx, w, req)
				require.Equal(t, tt.rCode, w.Result().StatusCode)
			}

			if tt.rCode == http.StatusAccepted {
				require.NoError(t, err)

				locationHeader := getAsyncLocationPath(sCtx, containerDataModel.TrackedResource.Location, "operationResults", req)
				require.NotNil(t, w.Header().Get("Location"))
				require.Equal(t, locationHeader, w.Header().Get("Location"))

				azureAsyncOpHeader := getAsyncLocationPath(sCtx, containerDataModel.TrackedResource.Location, "operationStatuses", req)
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
