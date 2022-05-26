// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package asyncoperation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/basedatamodel"
	base_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/queue"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/ucp/store"
)

const ResourceType = "Applications.Core/operationStatuses"

// asyncOperationsManager includes the necessary functions to manage asynchronous operations.
type asyncOperationsManager struct {
	storeClient  store.StorageClient
	enqueuer     queue.Enqueuer
	providerName string
	location     string
}

//go:generate mockgen -destination=./mock_manager.go -package=asyncoperation -self_package github.com/project-radius/radius/pkg/corerp/asyncoperation github.com/project-radius/radius/pkg/corerp/asyncoperation AsyncOperationsManager

// AsyncOperationsManager is the database interface for AsyncOperationStatus
type AsyncOperationsManager interface {
	Create(ctx context.Context, rootScope string, linkedResourceID string, operationName string, operationTimeout time.Duration) error
	Delete(ctx context.Context, rootScope string, operationID uuid.UUID) error
	Get(ctx context.Context, rootScope string, opID uuid.UUID) (*AsyncOperationStatus, error)
	Update(ctx context.Context, rootScope string, opID uuid.UUID, status basedatamodel.ProvisioningStates, endTime *time.Time, opErr *armerrors.ErrorDetails) error
}

// NewAsyncOperationsManager creates AsyncOperationsManagerInterface instance.
func NewAsyncOperationsManager(storeClient store.StorageClient, enqueuer queue.Enqueuer, providerName, location string) AsyncOperationsManager {
	return &asyncOperationsManager{
		storeClient:  storeClient,
		enqueuer:     enqueuer,
		providerName: providerName,
		location:     location,
	}
}

func (aom *asyncOperationsManager) operationStatusResourceID(rootScope string, operationID uuid.UUID) string {
	return fmt.Sprintf("%s/providers/%s/locations/%s/operationStatuses/%s", rootScope, aom.providerName, aom.location, operationID)
}

func (aom *asyncOperationsManager) Create(ctx context.Context, rootScope string, linkedResourceID string, operationName string, operationTimeout time.Duration) error {
	if aom.enqueuer == nil {
		return errors.New("enqueuer client is not set")
	}

	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	opID := aom.operationStatusResourceID(rootScope, sCtx.OperationID)

	aos := &AsyncOperationStatus{
		AsyncOperationStatus: armrpcv1.AsyncOperationStatus{
			ID:        opID,
			Name:      sCtx.OperationID.String(),
			Status:    basedatamodel.ProvisioningStateUpdating,
			StartTime: time.Now().UTC(),
		},
		LinkedResourceID: linkedResourceID,
		OperationName:    operationName,
		Location:         aom.location,
		HomeTenantID:     sCtx.HomeTenantID,
		ClientObjectID:   sCtx.ClientObjectID,
	}

	err := aom.storeClient.Save(ctx, &store.Object{
		Metadata: store.Metadata{ID: opID},
		Data:     aos,
	})

	if err != nil {
		return err
	}

	msg := &AsyncOperationRequestMessage{
		OperationID:           sCtx.OperationID,
		OperationName:         sCtx.OperationName,
		ResourceID:            aos.LinkedResourceID,
		CorrelationID:         sCtx.CorrelationID,
		TraceparentID:         sCtx.Traceparent,
		AcceptLanguage:        sCtx.AcceptLanguage,
		HomeTenantID:          sCtx.HomeTenantID,
		ClientObjectID:        sCtx.ClientObjectID,
		AsyncOperationBeginAt: time.Now().UTC(),
		AsyncOperationTimeout: operationTimeout,
	}

	err = aom.enqueuer.Enqueue(ctx, queue.NewMessage(msg))
	if err != nil {
		// We have to delete the operation from the DB
		// or we can update the Error for the operation.
		// But the client must be aware/warned that it needs to
		// kick off another request for the same operation.
		// aom.storeClient.Delete(ctx, opID)
		return err
	}

	return nil
}

func (aom *asyncOperationsManager) Get(ctx context.Context, rootScope string, operationID uuid.UUID) (*AsyncOperationStatus, error) {
	obj, err := aom.storeClient.Get(ctx, aom.operationStatusResourceID(rootScope, operationID))
	if err != nil {
		return nil, err
	}

	aos := &AsyncOperationStatus{}
	if err := base_ctrl.DecodeMap(obj.Data, aos); err != nil {
		return nil, err
	}

	return aos, nil
}

func (aom *asyncOperationsManager) Update(ctx context.Context, rootScope string, operationID uuid.UUID, status basedatamodel.ProvisioningStates, endTime *time.Time, opErr *armerrors.ErrorDetails) error {
	opID := aom.operationStatusResourceID(rootScope, operationID)

	dbObj, err := aom.storeClient.Get(ctx, opID)
	if err != nil {
		return err
	}

	aos := &AsyncOperationStatus{}
	if err := base_ctrl.DecodeMap(dbObj.Data, aos); err != nil {
		return err
	}

	aos.Status = status
	if opErr != nil {
		aos.Error = opErr
	}

	if endTime != nil {
		aos.EndTime = endTime
	}

	err = aom.storeClient.Save(ctx, &store.Object{Metadata: store.Metadata{ID: opID}, Data: aos}, store.WithETag(dbObj.ETag))

	if err != nil {
		return err
	}

	return nil
}

func (aom *asyncOperationsManager) Delete(ctx context.Context, rootScope string, operationID uuid.UUID) error {
	return aom.storeClient.Delete(ctx, aom.operationStatusResourceID(rootScope, operationID))
}
