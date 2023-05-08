/*
------------------------------------------------------------
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
------------------------------------------------------------
*/

package statusmanager

import (
	"context"
	"errors"
	"fmt"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/metrics"
	"github.com/project-radius/radius/pkg/trace"
	queue "github.com/project-radius/radius/pkg/ucp/queue/client"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"

	"github.com/google/uuid"
)

// statusManager includes the necessary functions to manage asynchronous operations.
type statusManager struct {
	storeClient  store.StorageClient
	queue        queue.Client
	providerName string
	location     string
}

// QueueOperationOptions is the options type provided when queueing an async operation.
type QueueOperationOptions struct {
	// OperationTimeout specifies the timeout duration for the async operation.
	OperationTimeout time.Duration
	// RetryAfter specifies the value of the Retry-After header that will be used for async operations.
	RetryAfter time.Duration
}

//go:generate mockgen -destination=./mock_statusmanager.go -package=statusmanager -self_package github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager StatusManager

// StatusManager is an interface to manage async operation status.
type StatusManager interface {
	// Get gets an async operation status object.
	Get(ctx context.Context, id resources.ID, operationID uuid.UUID) (*Status, error)
	// QueueAsyncOperation creates an async operation status object and queue async operation.
	QueueAsyncOperation(ctx context.Context, sCtx *v1.ARMRequestContext, options QueueOperationOptions) error
	// Update updates an async operation status.
	Update(ctx context.Context, id resources.ID, operationID uuid.UUID, state v1.ProvisioningState, endTime *time.Time, opError *v1.ErrorDetails) error
	// Delete deletes an async operation status.
	Delete(ctx context.Context, id resources.ID, operationID uuid.UUID) error
}

// New creates statusManager instance.
func New(storeClient store.StorageClient, q queue.Client, providerName, location string) StatusManager {
	return &statusManager{
		storeClient:  storeClient,
		queue:        q,
		providerName: providerName,
		location:     location,
	}
}

// operationStatusResourceID function is to build the operationStatus resourceID.
func (aom *statusManager) operationStatusResourceID(id resources.ID, operationID uuid.UUID) string {
	return fmt.Sprintf("%s/providers/%s/locations/%s/operationstatuses/%s", id.PlaneScope(), aom.providerName, aom.location, operationID)
}

func (aom *statusManager) QueueAsyncOperation(ctx context.Context, sCtx *v1.ARMRequestContext, options QueueOperationOptions) error {
	ctx, span := trace.StartProducerSpan(ctx, "statusmanager.QueueAsyncOperation publish", trace.FrontendTracerName)
	defer span.End()

	if aom.queue == nil {
		return errors.New("queue client is unset")
	}

	if sCtx == nil {
		return errors.New("*servicecontext.ARMRequestContext is unset")
	}

	opID := aom.operationStatusResourceID(sCtx.ResourceID, sCtx.OperationID)
	aos := &Status{
		AsyncOperationStatus: v1.AsyncOperationStatus{
			ID:        opID,
			Name:      sCtx.OperationID.String(),
			Status:    v1.ProvisioningStateAccepted,
			StartTime: time.Now().UTC(),
		},
		LinkedResourceID: sCtx.ResourceID.String(),
		Location:         aom.location,
		RetryAfter:       options.RetryAfter,
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

	if err = aom.queueRequestMessage(ctx, sCtx, aos, options.OperationTimeout); err != nil {
		delErr := aom.storeClient.Delete(ctx, opID)
		if delErr != nil {
			return delErr
		}
		return err
	}

	metrics.DefaultAsyncOperationMetrics.RecordQueuedAsyncOperation(ctx)

	return nil
}

func (aom *statusManager) Get(ctx context.Context, id resources.ID, operationID uuid.UUID) (*Status, error) {
	obj, err := aom.storeClient.Get(ctx, aom.operationStatusResourceID(id, operationID))
	if err != nil {
		return nil, err
	}

	aos := &Status{}
	if err := obj.As(&aos); err != nil {
		return nil, err
	}

	return aos, nil
}

func (aom *statusManager) Update(ctx context.Context, id resources.ID, operationID uuid.UUID, state v1.ProvisioningState, endTime *time.Time, opError *v1.ErrorDetails) error {
	opID := aom.operationStatusResourceID(id, operationID)

	obj, err := aom.storeClient.Get(ctx, opID)
	if err != nil {
		return err
	}

	s := &Status{}
	if err := obj.As(s); err != nil {
		return err
	}

	s.Status = state
	if endTime != nil {
		s.EndTime = endTime
	}

	if opError != nil {
		s.Error = opError
	}

	s.LastUpdatedTime = time.Now().UTC()

	obj.Data = s

	return aom.storeClient.Save(ctx, obj, store.WithETag(obj.ETag))
}

func (aom *statusManager) Delete(ctx context.Context, id resources.ID, operationID uuid.UUID) error {
	return aom.storeClient.Delete(ctx, aom.operationStatusResourceID(id, operationID))
}

// queueRequestMessage function is to put the async operation message to the queue to be worked on.
func (aom *statusManager) queueRequestMessage(ctx context.Context, sCtx *v1.ARMRequestContext, aos *Status, operationTimeout time.Duration) error {
	msg := &ctrl.Request{
		APIVersion:       sCtx.APIVersion,
		OperationID:      sCtx.OperationID,
		OperationType:    sCtx.OperationType,
		ResourceID:       aos.LinkedResourceID,
		CorrelationID:    sCtx.CorrelationID,
		TraceparentID:    trace.ExtractTraceparent(ctx),
		AcceptLanguage:   sCtx.AcceptLanguage,
		HomeTenantID:     sCtx.HomeTenantID,
		ClientObjectID:   sCtx.ClientObjectID,
		OperationTimeout: &operationTimeout,
	}

	return aom.queue.Enqueue(ctx, queue.NewMessage(msg))
}
