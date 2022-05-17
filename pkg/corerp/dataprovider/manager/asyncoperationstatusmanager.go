// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package manager

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/store"
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

func (osm *AsyncOperationStatusManager) Get(ctx context.Context, operationID uuid.UUID, rootScope string) (*datamodel.AsyncOperationStatus, error) {
	_, err := osm.storeClient.Get(ctx, operationID.String())
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (osm *AsyncOperationStatusManager) Create(ctx context.Context, operationID uuid.UUID, rootScope string, resourceID string, status string) error {
	// TODO: Create operation status
	return nil
}

func (osm *AsyncOperationStatusManager) Update(ctx context.Context, operationID uuid.UUID, rootScope string, status string, endTime *time.Time, error *armerrors.ErrorResponse) error {
	// TODO: Update operation status
	return nil
}

func (osm *AsyncOperationStatusManager) Delete(ctx context.Context, operationID uuid.UUID, rootScope string) error {
	// TODO: Delete operation status
	return nil
}
