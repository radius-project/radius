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
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/queue"
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
	Get(ctx context.Context, rootScope string, operationID uuid.UUID) (*AsyncOperationStatus, error)
	Update(ctx context.Context, rootScope string, operationID uuid.UUID, aos *AsyncOperationStatus) error
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

// operationStatusResourceID function is to build the operationStatus resourceID.
func (aom *asyncOperationsManager) operationStatusResourceID(rootScope string, operationID uuid.UUID) string {
	return fmt.Sprintf("%s/providers/%s/locations/%s/operationStatuses/%s", rootScope, aom.providerName, aom.location, operationID)
}

// Create function is to create an async operation status.
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

	err = aom.queueAsyncOperationRequestMessage(ctx, aos, operationTimeout)
	if err != nil {
		delErr := aom.storeClient.Delete(ctx, opID)
		if delErr != nil {
			return delErr
		}
		return err
	}

	return nil
}

// Get function is to get the requested async operation if it exists.
func (aom *asyncOperationsManager) Get(ctx context.Context, rootScope string, operationID uuid.UUID) (*AsyncOperationStatus, error) {
	obj, err := aom.storeClient.Get(ctx, aom.operationStatusResourceID(rootScope, operationID))
	if err != nil {
		return nil, err
	}

	aos := &AsyncOperationStatus{}
	if err := obj.As(&aos); err != nil {
		return nil, err
	}

	return aos, nil
}

// Update function is to update the existing async operation status.
func (aom *asyncOperationsManager) Update(ctx context.Context, rootScope string, operationID uuid.UUID, aos *AsyncOperationStatus) error {
	opID := aom.operationStatusResourceID(rootScope, operationID)

	obj, err := aom.storeClient.Get(ctx, opID)
	if err != nil {
		return err
	}

	obj.Data = aos

	return aom.storeClient.Save(ctx, obj, store.WithETag(obj.ETag))
}

// Delete function is to delete the async operation status.
func (aom *asyncOperationsManager) Delete(ctx context.Context, rootScope string, operationID uuid.UUID) error {
	return aom.storeClient.Delete(ctx, aom.operationStatusResourceID(rootScope, operationID))
}

// queueAsyncOperationRequestMessage function is to put the async operation message to the queue to be worked on.
func (aom *asyncOperationsManager) queueAsyncOperationRequestMessage(ctx context.Context, aos *AsyncOperationStatus, operationTimeout time.Duration) error {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	msg := &AsyncRequestMessage{
		OperationID:           sCtx.OperationID,
		OperationName:         sCtx.OperationName,
		ResourceID:            aos.LinkedResourceID,
		CorrelationID:         sCtx.CorrelationID,
		TraceparentID:         sCtx.Traceparent,
		AcceptLanguage:        sCtx.AcceptLanguage,
		HomeTenantID:          sCtx.HomeTenantID,
		ClientObjectID:        sCtx.ClientObjectID,
		AsyncOperationTimeout: operationTimeout,
	}

	return aom.enqueuer.Enqueue(ctx, queue.NewMessage(msg))
}
