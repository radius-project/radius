// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package manager

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/store"
	"github.com/project-radius/radius/pkg/util"
)

const ResourceType = "Applications.Core/operationStatuses"

// AsyncOperationStatusManager includes the helpers to manage the status of asynchronous operation.
type AsyncOperationStatusManager struct {
	storeClient store.StorageClient
}

// NewAsyncOperationStatusManager creates AsyncOperationStatusManager instance.
func NewAsyncOperationStatusManager(storeClient store.StorageClient) *AsyncOperationStatusManager {
	return &AsyncOperationStatusManager{storeClient: storeClient}
}

func (osm *AsyncOperationStatusManager) GetOperationResourceID(operationID uuid.UUID) string {
	// TODO: Generate Operation Status Resource ID
	return operationID.String()
}

func (osm *AsyncOperationStatusManager) Get(ctx context.Context, operationID uuid.UUID, rootScope string) (*datamodel.AsyncOperationStatus, error) {
	obj, err := osm.storeClient.Get(ctx, osm.GetOperationResourceID(operationID))
	if err != nil {
		return nil, err
	}

	dm := &datamodel.AsyncOperationStatus{}
	if err := util.DecodeMap(obj.Data, dm); err != nil {
		return nil, err
	}

	return dm, nil
}

func (osm *AsyncOperationStatusManager) Create(ctx context.Context, operationID uuid.UUID, operationName string, linkResourceID string, status string) error {
	opRID := osm.GetOperationResourceID(operationID)

	in := &datamodel.AsyncOperationStatus{
		AsyncOperationStatus: armrpcv1.AsyncOperationStatus{
			ID:        opRID,
			Name:      operationID.String(),
			Status:    string(basedatamodel.ProvisioningStateAccepted),
			StartTime: time.Now().UTC(),
		},
		LinkedResourceID: linkResourceID,
		OperationName:    operationName,
		Location:         "westus",
		ClientTenantID:   "",
		ClientObjectID:   "",
	}

	nr := &store.Object{
		Metadata: store.Metadata{ID: opRID},
		Data:     in,
	}

	_, err := osm.storeClient.Save(ctx, nr)
	if err != nil {
		return err
	}

	return nil
}

func (osm *AsyncOperationStatusManager) Update(ctx context.Context, operationID uuid.UUID, linkResourceID string, status basedatamodel.ProvisioningStates, endTime *time.Time, operationErr *armerrors.ErrorResponse) error {
	obj, err := osm.storeClient.Get(ctx, osm.GetOperationResourceID(operationID))
	if err != nil {
		return err
	}

	dm := &datamodel.AsyncOperationStatus{}
	if err := util.DecodeMap(obj.Data, dm); err != nil {
		return err
	}

	dm.Status = string(status)
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

	_, err = osm.storeClient.Save(ctx, nr, store.WithETag(obj.ETag))
	if err != nil {
		return err
	}

	return nil
}

func (osm *AsyncOperationStatusManager) Delete(ctx context.Context, operationID uuid.UUID, rootScope string) error {
	return osm.storeClient.Delete(ctx, osm.GetOperationResourceID(operationID))
}
