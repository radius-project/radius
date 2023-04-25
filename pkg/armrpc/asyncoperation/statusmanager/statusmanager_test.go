// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package statusmanager

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	queue "github.com/project-radius/radius/pkg/ucp/queue/client"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

type asyncOperationsManagerTest struct {
	manager     StatusManager
	storeClient *store.MockStorageClient
	queue       *queue.MockClient
}

const (
	operationTimeoutDuration      = time.Hour * 2
	opererationRetryAfterDuration = time.Second * 10
	azureEnvResourceID            = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0"
	ucpEnvResourceID              = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0"
	saveErr                       = "save error"
	enqueueErr                    = "enqueue error"
	deleteErr                     = "delete error"
	getErr                        = "get error"
)

func setup(tb testing.TB) (asyncOperationsManagerTest, *gomock.Controller) {
	ctrl := gomock.NewController(tb)
	sc := store.NewMockStorageClient(ctrl)
	enq := queue.NewMockClient(ctrl)
	aom := New(sc, enq, "Test-AsyncOperationsManager", "test-location")
	return asyncOperationsManagerTest{manager: aom, storeClient: sc, queue: enq}, ctrl
}

var reqCtx = &v1.ARMRequestContext{
	OperationID:    uuid.Must(uuid.NewRandom()),
	HomeTenantID:   "home-tenant-id",
	ClientObjectID: "client-object-id",
	OperationType:  "APPLICATIONS.CORE/ENVRIONMENTS|PUT",
	Traceparent:    "trace",
	AcceptLanguage: "lang",
}

var opID = uuid.New()

var testAos = &Status{
	AsyncOperationStatus: v1.AsyncOperationStatus{
		ID:        opID.String(),
		Name:      opID.String(),
		Status:    v1.ProvisioningStateUpdating,
		StartTime: time.Now().UTC(),
	},
	LinkedResourceID: uuid.New().String(),
	Location:         "test-location",
	RetryAfter:       opererationRetryAfterDuration,
	HomeTenantID:     "test-home-tenant-id",
	ClientObjectID:   "test-client-object-id",
}

func TestOperationStatusResourceID(t *testing.T) {
	resourceIDTests := []struct {
		resourceID          string
		operationID         uuid.UUID
		operationResourceID string
	}{
		{
			resourceID:          azureEnvResourceID,
			operationID:         uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			operationResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/providers/applications.core/locations/global/operationstatuses/00000000-0000-0000-0000-000000000001",
		}, {
			resourceID:          ucpEnvResourceID,
			operationID:         uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			operationResourceID: "/planes/radius/local/providers/applications.core/locations/global/operationstatuses/00000000-0000-0000-0000-000000000001",
		},
	}

	sm := &statusManager{providerName: "applications.core", location: v1.LocationGlobal}

	for _, tc := range resourceIDTests {
		t.Run(tc.resourceID, func(t *testing.T) {
			rid, err := resources.ParseResource(tc.resourceID)
			require.NoError(t, err)
			url := sm.operationStatusResourceID(rid, tc.operationID)
			require.Equal(t, tc.operationResourceID, url)
		})
	}
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
				aomTest.queue.EXPECT().Enqueue(gomock.Any(), gomock.Any(), gomock.Any()).Return(tt.EnqueueErr)
			}

			// If there is an error when enqueuing the message, the async operation should be deleted.
			if tt.EnqueueErr != nil {
				aomTest.storeClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(tt.DeleteErr)
			}

			options := QueueOperationOptions{
				OperationTimeout: operationTimeoutDuration,
				RetryAfter:       opererationRetryAfterDuration,
			}
			err := aomTest.manager.QueueAsyncOperation(context.TODO(), reqCtx, options)

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
			rid, err := resources.ParseResource(azureEnvResourceID)
			require.NoError(t, err)
			err = aomTest.manager.Delete(context.TODO(), rid, uuid.New())

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

			rid, err := resources.ParseResource(azureEnvResourceID)
			require.NoError(t, err)
			aos, err := aomTest.manager.Get(context.TODO(), rid, uuid.New())

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

			testAos.Status = v1.ProvisioningStateSucceeded
			rid, err := resources.ParseResource(azureEnvResourceID)
			require.NoError(t, err)
			err = aomTest.manager.Update(context.TODO(), rid, opID, v1.ProvisioningStateAccepted, nil, nil)

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
