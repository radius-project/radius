/*
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
*/

// Package awsresources contains the UCP backend controller that finishes AWS resource operations. UCP owns every
// create, update, and delete of an AWS resource: the frontend starts it in AWS Cloud Control and queues an operation,
// and this controller waits for AWS and then updates the resource's record.
package awsresources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/radius-project/radius/pkg/components/database"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_aws "github.com/radius-project/radius/pkg/ucp/resources/aws"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*AwaitController)(nil)

// AwaitController waits for an AWS resource operation to finish and updates the record. It polls Cloud Control with
// the request token saved on the record. Polling only reads AWS, so a redelivered message or a restarted worker runs
// it again safely.
type AwaitController struct {
	ctrl.BaseController
	clients ucp_aws.Clients
	method  v1.OperationMethod
}

// NewAwaitController creates an AwaitController for PUT or DELETE.
func NewAwaitController(opts ctrl.Options, clients ucp_aws.Clients, method v1.OperationMethod) (ctrl.Controller, error) {
	if clients.CloudControl == nil {
		return nil, errors.New("the AWS Cloud Control client is required")
	}
	return &AwaitController{
		BaseController: ctrl.NewBaseAsyncController(opts),
		clients:        clients,
		method:         method,
	}, nil
}

// Register registers an AwaitController for PUT and DELETE under the AWS resource operation type.
func Register(registry *worker.ControllerRegistry, clients ucp_aws.Clients, opts ctrl.Options) error {
	var err error
	for _, method := range []v1.OperationMethod{v1.OperationPut, v1.OperationDelete} {
		err = errors.Join(err, registry.Register(records.OperationType, method, func(opts ctrl.Options) (ctrl.Controller, error) {
			return NewAwaitController(opts, clients, method)
		}, opts))
	}
	return err
}

// heartbeatInterval is how often the worker saves the record while it polls AWS, so the operation does not look
// stale. Tests shorten it.
var heartbeatInterval = records.PollInterval

// maxSaveAttempts is how often the worker retries a save that loses a race with another write before it requeues.
const maxSaveAttempts = 3

// Run waits for the operation to finish and updates the record. When the record cannot be read or saved, it requeues
// the operation, so the worker runs it again later and the operation stays in progress until the record is saved.
func (c *AwaitController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	id, err := resources.ParseResource(request.ResourceID)
	if err != nil {
		return ctrl.NewFailedResult(v1.ErrorDetails{Code: v1.CodeInvalid, Message: err.Error()}), nil
	}

	store := records.NewStore(c.DatabaseClient())
	rec, _, err := store.Get(ctx, id)
	if err != nil {
		// Without the record, UCP cannot tell which AWS request to poll.
		return requeue(ctx, err), nil
	}
	if rec == nil {
		if c.method == v1.OperationDelete {
			// The record is already gone, so the delete is done.
			return ctrl.Result{}, nil
		}
		return ctrl.NewFailedResult(v1.ErrorDetails{
			Code:    v1.CodeInternal,
			Message: fmt.Sprintf("UCP has no record of the AWS resource %q.", id.Name()),
			Target:  request.ResourceID,
		}), nil
	}

	op := records.PendingOperation(rec)
	if c.method == v1.OperationDelete {
		op = records.OperationDelete
	}
	requestToken := rec.Properties.RequestToken
	if requestToken == "" {
		return ctrl.NewFailedResult(v1.ErrorDetails{
			Code:    v1.CodeInternal,
			Message: fmt.Sprintf("The record of the AWS resource %q has no AWS request to wait for. Deploy again.", id.Name()),
			Target:  request.ResourceID,
		}), nil
	}

	// Save the record while waiting, so a request does not take the operation for a stale one.
	if err := store.Touch(ctx, id); err != nil {
		logger.Error(err, "Failed to save the record of a running AWS operation", "alias", id.Name())
	}
	stop := heartbeat(ctx, store, id, heartbeatInterval)
	logger.Info("Waiting for AWS Cloud Control", "alias", id.Name(), "awsIdentifier", rec.Properties.AWSIdentifier, "operation", op)
	status, err := ucp_aws.WaitForRequest(ctx, c.clients.CloudControl, id.FindScope(resources_aws.ScopeRegions), requestToken)
	stop()

	var notFound *cctypes.RequestTokenNotFoundException
	if errors.As(err, &notFound) {
		// Cloud Control no longer knows the request, so UCP cannot tell how it ended. Forget it: the next deploy reads
		// the resource from AWS, or sends a first create again with the record's client token.
		if saveErr := update(ctx, store, id, func(rec *datamodel.AWSResource) bool {
			records.Fail(rec)
			return false
		}); saveErr != nil {
			return requeue(ctx, saveErr), nil
		}
		return ctrl.NewFailedResult(v1.ErrorDetails{
			Code:    v1.CodeInternal,
			Message: fmt.Sprintf("AWS Cloud Control no longer has the request for the AWS resource %q. Deploy again.", id.Name()),
			Target:  request.ResourceID,
		}), nil
	}

	var failed *ucp_aws.RequestStatus
	if err != nil && !errors.As(err, &failed) {
		// Polling failed, or the worker canceled the context on timeout or shutdown. The record keeps its request
		// token, so the worker or the next request polls the same AWS request again.
		if ctx.Err() != nil {
			return ctrl.Result{}, err
		}
		return requeue(ctx, err), nil
	}

	if failed != nil {
		if saveErr := update(ctx, store, id, func(rec *datamodel.AWSResource) bool {
			// A failed create removes the record, so the next deploy starts over.
			if op == records.OperationCreate {
				return true
			}
			records.Fail(rec)
			return false
		}); saveErr != nil {
			return requeue(ctx, saveErr), nil
		}
		code := v1.CodeInternal
		if failed.IsClientError() {
			code = v1.CodeInvalid
		}
		return ctrl.NewFailedResult(v1.ErrorDetails{Code: code, Message: failed.Error(), Target: request.ResourceID}), nil
	}

	err = update(ctx, store, id, func(rec *datamodel.AWSResource) bool {
		if op == records.OperationDelete {
			return true
		}
		if status.Identifier != "" {
			rec.Properties.AWSIdentifier = status.Identifier
		}
		records.ApplyPending(rec)
		return false
	})
	if err != nil {
		return requeue(ctx, fmt.Errorf("AWS finished the %s of %q, but UCP failed to save the result: %w", strings.ToLower(op), id.Name(), err)), nil
	}
	return ctrl.Result{}, nil
}

// update reads the record, applies change, and saves it. When change returns true, it removes the record instead. A
// save that loses a race with another write, such as the heartbeat, is tried again.
func update(ctx context.Context, store *records.Store, id resources.ID, change func(rec *datamodel.AWSResource) bool) error {
	for range maxSaveAttempts {
		rec, etag, err := store.Get(ctx, id)
		if err != nil {
			return err
		} else if rec == nil {
			return nil
		}
		if change(rec) {
			return store.Delete(ctx, id)
		}
		_, err = store.Save(ctx, rec, etag)
		if errors.Is(err, &database.ErrConcurrency{}) {
			continue
		}
		return err
	}
	return errors.New("the record kept changing")
}

// heartbeat saves the record every interval until the returned function is called.
func heartbeat(ctx context.Context, store *records.Store, id resources.ID, interval time.Duration) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := store.Touch(ctx, id); err != nil && ctx.Err() == nil {
					ucplog.FromContextOrDiscard(ctx).Error(err, "Failed to save the record of a running AWS operation", "alias", id.Name())
				}
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

// requeue returns a result that keeps the operation in progress and has the worker run it again later.
func requeue(ctx context.Context, err error) ctrl.Result {
	ucplog.FromContextOrDiscard(ctx).Error(err, "Retrying the AWS operation later")
	result := ctrl.Result{Requeue: true}
	result.SetProvisioningState(v1.ProvisioningStateUpdating)
	return result
}
