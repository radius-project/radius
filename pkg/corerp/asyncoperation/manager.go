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

// manager includes the necessary functions to manage asynchronous operations.
type manager struct {
	storeClient  store.StorageClient
	enqueuer     queue.Enqueuer
	providerName string
	location     string
}

//go:generate mockgen -destination=./mock_manager.go -package=asyncoperation -self_package github.com/project-radius/radius/pkg/corerp/asyncoperation github.com/project-radius/radius/pkg/corerp/asyncoperation AsyncOperationsManager

// Manager is the database interface for AsyncOperationStatus
type Manager interface {
	Create(ctx context.Context, rootScope string, linkedResourceID string, operationName string, operationTimeout time.Duration) error
	Delete(ctx context.Context, rootScope string, operationID uuid.UUID) error
	Get(ctx context.Context, rootScope string, operationID uuid.UUID) (*Status, error)
	Update(ctx context.Context, rootScope string, operationID uuid.UUID, aos *Status) error
}

// NewManager creates manager instance.
func NewManager(storeClient store.StorageClient, enqueuer queue.Enqueuer, providerName, location string) Manager {
	return &manager{
		storeClient:  storeClient,
		enqueuer:     enqueuer,
		providerName: providerName,
		location:     location,
	}
}

// operationStatusResourceID function is to build the operationStatus resourceID.
func (aom *manager) operationStatusResourceID(rootScope string, operationID uuid.UUID) string {
	return fmt.Sprintf("%s/providers/%s/locations/%s/operationStatuses/%s", rootScope, aom.providerName, aom.location, operationID)
}

// Create function is to create an async operation status.
func (aom *manager) Create(ctx context.Context, rootScope string, linkedResourceID string, operationName string, operationTimeout time.Duration) error {
	if aom.enqueuer == nil {
		return errors.New("enqueuer client is not set")
	}

	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	opID := aom.operationStatusResourceID(rootScope, sCtx.OperationID)

	aos := &Status{
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

	err = aom.queueRequestMessage(ctx, aos, operationTimeout)
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
func (aom *manager) Get(ctx context.Context, rootScope string, operationID uuid.UUID) (*Status, error) {
	obj, err := aom.storeClient.Get(ctx, aom.operationStatusResourceID(rootScope, operationID))
	if err != nil {
		return nil, err
	}

	aos := &Status{}
	if err := obj.As(&aos); err != nil {
		return nil, err
	}

	return aos, nil
}

// Update function is to update the existing async operation status.
func (aom *manager) Update(ctx context.Context, rootScope string, operationID uuid.UUID, aos *Status) error {
	opID := aom.operationStatusResourceID(rootScope, operationID)

	obj, err := aom.storeClient.Get(ctx, opID)
	if err != nil {
		return err
	}

	s := &Status{}
	if err := obj.As(s); err != nil {
		return err
	}

	s.Status = aos.Status
	s.EndTime = aos.EndTime
	if aos.Error != nil {
		s.Error = aos.Error
	}

	return aom.storeClient.Save(ctx, obj, store.WithETag(obj.ETag))
}

// Delete function is to delete the async operation status.
func (aom *manager) Delete(ctx context.Context, rootScope string, operationID uuid.UUID) error {
	return aom.storeClient.Delete(ctx, aom.operationStatusResourceID(rootScope, operationID))
}

// queueRequestMessage function is to put the async operation message to the queue to be worked on.
func (aom *manager) queueRequestMessage(ctx context.Context, aos *Status, operationTimeout time.Duration) error {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	msg := &Request{
		OperationID:      sCtx.OperationID,
		OperationName:    sCtx.OperationName,
		ResourceID:       aos.LinkedResourceID,
		CorrelationID:    sCtx.CorrelationID,
		TraceparentID:    sCtx.Traceparent,
		AcceptLanguage:   sCtx.AcceptLanguage,
		HomeTenantID:     sCtx.HomeTenantID,
		ClientObjectID:   sCtx.ClientObjectID,
		OperationTimeout: operationTimeout,
	}

	return aom.enqueuer.Enqueue(ctx, queue.NewMessage(msg))
}
