// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package asyncoperation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/queue"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

type asyncOperationsManagerTest struct {
	manager     StatusManager
	storeClient *store.MockStorageClient
	enqueuer    *queue.MockEnqueuer
}

const (
	operationTimeoutDuration = time.Hour * 2
	testRootScope            = "test-root-scope"
	saveErr                  = "save error"
	enqueueErr               = "enqueue error"
	deleteErr                = "delete error"
	getErr                   = "get error"
)

func setup(tb testing.TB) (asyncOperationsManagerTest, *gomock.Controller) {
	ctrl := gomock.NewController(tb)
	sc := store.NewMockStorageClient(ctrl)
	enq := queue.NewMockEnqueuer(ctrl)
	aom := NewStatusManager(sc, enq, "Test-AsyncOperationsManager", "test-location")
	return asyncOperationsManagerTest{manager: aom, storeClient: sc, enqueuer: enq}, ctrl
}

var reqCtx = &servicecontext.ARMRequestContext{
	OperationID:    uuid.Must(uuid.NewRandom()),
	HomeTenantID:   "home-tenant-id",
	ClientObjectID: "client-object-id",
	OperationType:  "APPLICATIONS.CORE/ENVRIONMENTS|PUT",
	Traceparent:    "trace",
	AcceptLanguage: "lang",
}

var opID = uuid.New()

var testAos = &Status{
	AsyncOperationStatus: armrpcv1.AsyncOperationStatus{
		ID:        opID.String(),
		Name:      opID.String(),
		Status:    basedatamodel.ProvisioningStateUpdating,
		StartTime: time.Now().UTC(),
	},
	LinkedResourceID: uuid.New().String(),
	Location:         "test-location",
	HomeTenantID:     "test-home-tenant-id",
	ClientObjectID:   "test-client-object-id",
}

func TestCreateAsyncOperationStatus(t *testing.T) {
	createCases := []struct {
		Desc       string
		SaveErr    error
		EnqueueErr error
		DeleteErr  error
	}{
		{
			Desc:       "create_success",
			SaveErr:    nil,
			EnqueueErr: nil,
			DeleteErr:  nil,
		},
		{
			Desc:       "create_save-error",
			SaveErr:    fmt.Errorf(saveErr),
			EnqueueErr: nil,
			DeleteErr:  nil,
		},
		{
			Desc:       "create_enqueue-error",
			SaveErr:    nil,
			EnqueueErr: fmt.Errorf(enqueueErr),
			DeleteErr:  nil,
		},
		{
			Desc:       "create_delete-error",
			SaveErr:    nil,
			EnqueueErr: fmt.Errorf(enqueueErr),
			DeleteErr:  fmt.Errorf(deleteErr),
		},
	}

	for _, tt := range createCases {
		t.Run(fmt.Sprint(tt.Desc), func(t *testing.T) {
			aomTest, mctrl := setup(t)
			defer mctrl.Finish()

			aomTest.storeClient.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(tt.SaveErr)

			// We can't expect an async operation to be queued if it is not saved to the DB.
			if tt.SaveErr == nil {
				aomTest.enqueuer.EXPECT().Enqueue(gomock.Any(), gomock.Any(), gomock.Any()).Return(tt.EnqueueErr)
			}

			// If there is an error when enqueuing the message, the async operation should be deleted.
			if tt.EnqueueErr != nil {
				aomTest.storeClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(tt.DeleteErr)
			}

			err := aomTest.manager.QueueAsyncOperation(context.TODO(), reqCtx, operationTimeoutDuration)

			if tt.SaveErr == nil && tt.EnqueueErr == nil && tt.DeleteErr == nil {
				require.NoError(t, err)
			}

			if tt.SaveErr != nil {
				require.Error(t, err, saveErr)
			}

			if tt.EnqueueErr != nil {
				require.Error(t, err, enqueueErr)
			}

			if tt.DeleteErr != nil {
				require.Error(t, err, deleteErr)
			}
		})
	}
}

func TestDeleteAsyncOperationStatus(t *testing.T) {
	deleteCases := []struct {
		Desc      string
		DeleteErr error
	}{
		{
			Desc:      "delete_success",
			DeleteErr: nil,
		},
		{
			Desc:      "delete_error",
			DeleteErr: fmt.Errorf(deleteErr),
		},
	}

	for _, tt := range deleteCases {
		t.Run(fmt.Sprint(tt.Desc), func(t *testing.T) {
			aomTest, mctrl := setup(t)
			defer mctrl.Finish()

			aomTest.storeClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(tt.DeleteErr)

			err := aomTest.manager.Delete(context.TODO(), testRootScope, uuid.New())

			if tt.DeleteErr != nil {
				require.Error(t, err, deleteErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetAsyncOperationStatus(t *testing.T) {
	getCases := []struct {
		Desc   string
		GetErr error
		Obj    *store.Object
	}{
		{
			Desc:   "get_success",
			GetErr: nil,
			Obj: &store.Object{
				Metadata: store.Metadata{ID: opID.String(), ETag: "etag"},
				Data:     testAos,
			},
		},
		{
			Desc:   "create_enqueue-error",
			GetErr: fmt.Errorf(getErr),
			Obj:    nil,
		},
	}

	for _, tt := range getCases {
		t.Run(fmt.Sprint(tt.Desc), func(t *testing.T) {
			aomTest, mctrl := setup(t)
			defer mctrl.Finish()

			aomTest.storeClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(tt.Obj, tt.GetErr)

			aos, err := aomTest.manager.Get(context.TODO(), testRootScope, uuid.New())

			if tt.GetErr == nil {
				require.NoError(t, err)
				expected := &Status{}
				_ = tt.Obj.As(&expected)
				require.Equal(t, expected, aos)
			}

			if tt.GetErr != nil {
				require.Error(t, err, getErr)
			}
		})
	}
}

func TestUpdateAsyncOperationStatus(t *testing.T) {
	updateCases := []struct {
		Desc    string
		GetErr  error
		Obj     *store.Object
		SaveErr error
	}{
		{
			Desc:   "update_success",
			GetErr: nil,
			Obj: &store.Object{
				Metadata: store.Metadata{ID: opID.String(), ETag: "etag"},
				Data:     testAos,
			},
			SaveErr: nil,
		},
	}

	for _, tt := range updateCases {
		t.Run(fmt.Sprint(tt.Desc), func(t *testing.T) {
			aomTest, mctrl := setup(t)
			defer mctrl.Finish()

			aomTest.storeClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(tt.Obj, tt.GetErr)

			if tt.GetErr == nil {
				aomTest.storeClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(tt.SaveErr)
			}

			testAos.Status = basedatamodel.ProvisioningStateSucceeded

			err := aomTest.manager.Update(context.TODO(), testRootScope, opID, basedatamodel.ProvisioningStateAccepted, nil, nil)

			if tt.GetErr == nil && tt.SaveErr == nil {
				require.NoError(t, err)
			}

			if tt.GetErr != nil {
				require.Error(t, err, getErr)
			}

			if tt.SaveErr != nil {
				require.Error(t, err, saveErr)
			}
		})
	}
}
