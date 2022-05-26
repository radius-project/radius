// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package asyncoperation

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/queue"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

type asyncOperationsManagerTest struct {
	manager     AsyncOperationsManager
	storeClient *store.MockStorageClient
	enqueuer    *queue.MockEnqueuer
}

const (
	operationTimeoutDuration = time.Hour * 2
	testRootScope            = "test-root-scope"
)

func setup(tb testing.TB) asyncOperationsManagerTest {
	ctrl := gomock.NewController(tb)
	sc := store.NewMockStorageClient(ctrl)
	enq := queue.NewMockEnqueuer(ctrl)
	aom := NewAsyncOperationsManager(sc, enq, "Test-AsyncOperationsManager", "test-location")
	return asyncOperationsManagerTest{manager: aom, storeClient: sc, enqueuer: enq}
}

func TestCreate_Success(t *testing.T) {
	aomTest := setup(t)

	ctx := servicecontext.WithARMRequestContext(context.Background(), &servicecontext.ARMRequestContext{
		OperationID:    uuid.Must(uuid.NewRandom()),
		HomeTenantID:   "home-tenant-id",
		ClientObjectID: "client-object-id",
		OperationName:  "op-name",
		Traceparent:    "trace",
		AcceptLanguage: "lang",
	})

	aomTest.storeClient.
		EXPECT().
		Save(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
			return nil
		})

	aomTest.enqueuer.
		EXPECT().
		Enqueue(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, msg *queue.Message, options ...queue.EnqueueOptions) error {
			return nil
		})

	err := aomTest.manager.Create(ctx, testRootScope, "linked-resource-id", "operation-name", operationTimeoutDuration)

	require.NoError(t, err)
}
