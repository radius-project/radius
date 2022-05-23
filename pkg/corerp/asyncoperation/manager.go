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

type AsyncOperationManagerInterface interface {
	Get(ctx context.Context, rootScope string, operationID uuid.UUID) (*AsyncOperationStatus, error)
	Create(ctx context.Context, rootScope string, operationID uuid.UUID, operationName string, linkResourceID string, status string, operationTimeout time.Duration) error
	Update(ctx context.Context, rootScope string, operationID uuid.UUID, status basedatamodel.ProvisioningStates, endTime *time.Time, operationErr *armerrors.ErrorDetails) error
	Delete(ctx context.Context, rootScope string, operationID uuid.UUID) error
}

// AsyncOperationManager includes the helpers to manage the status of asynchronous operation.
type AsyncOperationManager struct {
	storeClient  store.StorageClient
	enqueuer     queue.Enqueuer
	providerName string
	location     string
}

// NewAsyncOperationManager creates AsyncOperationManager instance.
func NewAsyncOperationManager(storeClient store.StorageClient, enqueuer queue.Enqueuer, providerName, location string) AsyncOperationManagerInterface {
	return &AsyncOperationManager{
		storeClient:  storeClient,
		enqueuer:     enqueuer,
		providerName: providerName,
		location:     location,
	}
}

func (osm *AsyncOperationManager) operationStatusResourceID(rootScope string, operationID uuid.UUID) string {
	return fmt.Sprintf("%s/providers/%s/locations/%s/operationStatuses/%s", rootScope, osm.providerName, osm.location, operationID)
}

func (osm *AsyncOperationManager) Get(ctx context.Context, rootScope string, operationID uuid.UUID) (*AsyncOperationStatus, error) {
	obj, err := osm.storeClient.Get(ctx, osm.operationStatusResourceID(rootScope, operationID))
	if err != nil {
		return nil, err
	}

	dm := &AsyncOperationStatus{}
	if err := base_ctrl.DecodeMap(obj.Data, dm); err != nil {
		return nil, err
	}

	return dm, nil
}

func (osm *AsyncOperationManager) Create(ctx context.Context, rootScope string, operationID uuid.UUID, operationName string, linkResourceID string, status string, operationTimeout time.Duration) error {
	if osm.enqueuer == nil {
		return errors.New("enqueuer client is not set")
	}

	sCtx := servicecontext.ARMRequestContextFromContext(ctx)
	opRID := osm.operationStatusResourceID(rootScope, operationID)

	// TODO: We should check if there is an ongoing async operation for the resource
	// TODO: Would it be okay to have an asyncOperation field on the resource? And if
	// TODO: there is an operation on the resource we can update that field?

	opStatus := &AsyncOperationStatus{
		AsyncOperationStatus: armrpcv1.AsyncOperationStatus{
			ID:        opRID,
			Name:      operationID.String(),
			Status:    basedatamodel.ProvisioningStateUpdating,
			StartTime: time.Now().UTC(),
		},
		LinkedResourceID: linkResourceID,
		OperationName:    operationName,
		Location:         osm.location,
		HomeTenantID:     sCtx.HomeTenantID,
		ClientObjectID:   sCtx.ClientObjectID,
	}

	err := osm.storeClient.Save(ctx, &store.Object{
		Metadata: store.Metadata{ID: opRID},
		Data:     opStatus,
	})

	if err != nil {
		return err
	}

	msg := &AsyncOperationRequestMessage{
		OperationID:           operationID,
		OperationName:         sCtx.OperationName,
		ResourceID:            opStatus.LinkedResourceID,
		CorrelationID:         sCtx.CorrelationID,
		TraceparentID:         sCtx.Traceparent,
		AcceptLanguage:        sCtx.AcceptLanguage,
		HomeTenantID:          sCtx.HomeTenantID,
		ClientObjectID:        sCtx.ClientObjectID,
		AsyncOperationBeginAt: time.Now().UTC(),
		AsyncOperationTimeout: operationTimeout,
	}

	return osm.enqueuer.Enqueue(ctx, queue.NewMessage(msg))
}

func (osm *AsyncOperationManager) Update(ctx context.Context, rootScope string, operationID uuid.UUID, status basedatamodel.ProvisioningStates, endTime *time.Time, operationErr *armerrors.ErrorDetails) error {
	obj, err := osm.storeClient.Get(ctx, osm.operationStatusResourceID(rootScope, operationID))
	if err != nil {
		return err
	}

	dm := &AsyncOperationStatus{}
	if err := base_ctrl.DecodeMap(obj.Data, dm); err != nil {
		return err
	}

	dm.Status = status
	if operationErr != nil {
		dm.Error = operationErr
	}

	if endTime != nil {
		dm.EndTime = endTime
	}

	nr := &store.Object{
		Metadata: store.Metadata{ID: dm.ID},
		Data:     dm,
	}

	err = osm.storeClient.Save(ctx, nr, store.WithETag(obj.ETag))
	if err != nil {
		return err
	}

	return nil
}

func (osm *AsyncOperationManager) Delete(ctx context.Context, rootScope string, operationID uuid.UUID) error {
	return osm.storeClient.Delete(ctx, osm.operationStatusResourceID(rootScope, operationID))
}
