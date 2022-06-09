// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func TestCreateOrUpdateContainerRun_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mds := store.NewMockStorageClient(mctrl)
	msm := statusmanager.NewMockStatusManager(mctrl)

	ctx := context.Background()

	nrCases := []struct {
		desc    string
		headers map[string]string
		getObj  *store.Object
		getErr  error
		saveErr error
		opID    string
		qErr    error
		rCode   int
		rErr    error
	}{
		{
			"async-create-new-container-success",
			map[string]string{},
			nil,
			&store.ErrNotFound{},
			nil,
			uuid.New().String(),
			nil,
			http.StatusCreated,
			nil,
		},
		{
			"async-create-new-container-concurrency-error",
			map[string]string{},
			nil,
			&store.ErrConcurrency{},
			nil,
			uuid.New().String(),
			nil,
			http.StatusCreated,
			&store.ErrConcurrency{},
		},
		{
			"async-create-new-container-enqueue-error",
			map[string]string{},
			nil,
			&store.ErrConcurrency{},
			nil,
			"",
			errors.New("enqueuer client is unset"),
			http.StatusInternalServerError,
			errors.New("enqueuer client is unset"),
		},
	}

	for _, tt := range nrCases {
		t.Run(tt.desc, func(t *testing.T) {
			containerInput, _, _ := getTestModels20220315privatepreview()

			w := httptest.NewRecorder()
			req, err := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodPut, testHeaderfile, containerInput)
			require.NoError(t, err)

			ctx := radiustesting.ARMTestContextFromRequest(req)
			sCtx := servicecontext.ARMRequestContextFromContext(ctx)

			// Conditional Mocking
			mds.EXPECT().Get(gomock.Any(), gomock.Any()).Return(tt.getObj, tt.getErr).MaxTimes(1)

			if tt.getErr == nil || errors.Is(&store.ErrNotFound{}, tt.getErr) {
				mds.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(tt.saveErr).MaxTimes(1)

				if tt.saveErr == nil {
					msm.EXPECT().QueueAsyncOperation(gomock.Any(), gomock.Any(), gomock.Any()).Return(tt.qErr).MaxTimes(1)
				}
			}

			ctl, err := NewCreateOrUpdateContainer(mds, msm)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, req)
			if tt.rErr != nil {
				require.Error(t, tt.rErr)
			} else {
				require.NoError(t, err)

				_ = resp.Apply(ctx, w, req)
				require.Equal(t, tt.rCode, w.Result().StatusCode)

				require.NotNil(t, w.Header().Get("Location"))
				require.Equal(t, GetOperationResultPath(req, sCtx.OperationID.String()),
					w.Header().Get("Location"))

				// FIXME: "/subscriptions//providers//locations//operationsStatuses/135dd863-6c0a-4677-8429-b99b5e324f99"
				require.NotNil(t, w.Header().Get("Azure-AsyncOperation"))
				require.Equal(t, GetOperationStatusPath(req, sCtx.OperationID.String()),
					w.Header().Get("Azure-AsyncOperation"))
			}
		})
	}
}
