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

package awsresources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_aws "github.com/radius-project/radius/pkg/ucp/resources/aws"
)

// now returns the current time. Tests replace it.
var now = time.Now

// timedOut means the handler's timeout passed before the operation finished.
type timedOut struct {
	timeout  time.Duration
	progress native.Progress
}

func (t *timedOut) Error() string {
	return fmt.Sprintf("the operation did not finish within %s", t.timeout)
}

// waitNative calls the native handler's Progress until the operation finishes, ctx ends, or the handler's timeout
// passes. The timeout counts from when this worker started waiting, so a restarted worker waits it again. Progress
// only reads AWS, so waiting again changes nothing in AWS.
func (c *AwaitController) waitNative(ctx context.Context, id resources.ID, identifier string, op string) (native.Progress, error) {
	t := c.clients.Native.Lookup(resources_aws.ToAWSResourceType(id))
	if t == nil {
		return native.Progress{}, &native.Failure{Message: fmt.Sprintf("A native handler started the operation on the AWS resource %q, but UCP has no native handler for %s. Deploy again.", id.Name(), resources_aws.ToAWSResourceType(id))}
	}
	if identifier == "" {
		return native.Progress{}, &native.Failure{Message: fmt.Sprintf("UCP has no AWS identifier for the AWS resource %q, so it cannot wait for it. Deploy again.", id.Name())}
	}

	region := id.FindScope(resources_aws.ScopeRegions)
	nativeOp := native.Operation(op)
	timeout := t.Handler.Options().Timeout(nativeOp)
	deadline := now().Add(timeout)
	interval := ucp_aws.MinPollInterval
	for {
		progress, err := t.Handler.Progress(ctx, region, identifier, nativeOp)
		if errors.Is(err, native.ErrNotFound) {
			if nativeOp == native.OperationDelete {
				return native.Progress{Done: true}, nil
			}
			return native.Progress{Done: true, Failure: &native.Failure{Message: fmt.Sprintf("AWS no longer has the AWS resource %q. Deploy again to create it.", id.Name())}}, nil
		} else if err != nil {
			return native.Progress{}, err
		}
		if progress.Done {
			return progress, nil
		}
		if !now().Before(deadline) {
			return progress, &timedOut{timeout: timeout, progress: progress}
		}

		select {
		case <-ctx.Done():
			return native.Progress{}, fmt.Errorf("stopped waiting for the AWS resource %q: %w", id.Name(), ctx.Err())
		case <-time.After(interval):
		}
		interval = min(interval*2, ucp_aws.MaxPollInterval)
	}
}

// finishNative saves the result of a native operation on the record, as Run does for Cloud Control:
//
//   - success: a delete removes the record, and a create or update makes its pending values last-applied.
//   - failure, or a missing handler: the pending values are dropped. A failed create keeps its client token and
//     identifier, because the resource may exist in AWS: the next deploy reads it, and a DELETE can remove it.
//   - timeout: the operation fails, naming the state UCP waited for. An update's pending values may be applied, so
//     Radius owns them. The next deploy reads the resource and continues.
//   - polling failed or stopped: the operation stays in progress, and the worker or the next request waits again.
func finishNative(ctx context.Context, store *records.Store, id resources.ID, request *ctrl.Request, op string, progress native.Progress, err error) (ctrl.Result, error) {
	var timeout *timedOut
	var failure *native.Failure
	switch {
	case errors.As(err, &timeout):
		details := v1.ErrorDetails{
			Code:    v1.CodeInternal,
			Message: timeoutMessage(id, op, timeout),
			Target:  request.ResourceID,
		}
		return saveNativeFailure(ctx, store, id, request, details, func(rec *datamodel.AWSResource) {
			if op == records.OperationUpdate {
				records.KeepPending(rec)
			}
		})
	case errors.As(err, &failure):
		return saveNativeFailure(ctx, store, id, request, failureOf(failure, request), nil)
	case err != nil:
		if ctx.Err() != nil {
			return ctrl.Result{}, err
		}
		return requeue(ctx, err), nil
	case progress.Failure != nil:
		return saveNativeFailure(ctx, store, id, request, failureOf(progress.Failure, request), nil)
	}

	err = update(ctx, store, id, request.OperationID, func(rec *datamodel.AWSResource) bool {
		if op == records.OperationDelete {
			return true
		}
		records.ApplyPending(rec)
		return false
	})
	if errors.Is(err, errSuperseded) {
		return superseded(id), nil
	} else if err != nil {
		return requeue(ctx, fmt.Errorf("AWS finished the %s of %q, but UCP failed to save the result: %w", strings.ToLower(op), id.Name(), err)), nil
	}
	return ctrl.Result{}, nil
}

// saveNativeFailure finishes the record's operation with failure. before, when set, changes the record first.
func saveNativeFailure(ctx context.Context, store *records.Store, id resources.ID, request *ctrl.Request, failure v1.ErrorDetails, before func(rec *datamodel.AWSResource)) (ctrl.Result, error) {
	err := update(ctx, store, id, request.OperationID, func(rec *datamodel.AWSResource) bool {
		if before != nil {
			before(rec)
		}
		records.Fail(rec, &failure)
		return false
	})
	if errors.Is(err, errSuperseded) {
		return superseded(id), nil
	} else if err != nil {
		return requeue(ctx, err), nil
	}
	return ctrl.NewFailedResult(failure), nil
}

func timeoutMessage(id resources.ID, op string, t *timedOut) string {
	message := fmt.Sprintf("UCP waited %s for the %s of the AWS resource %q to finish", t.timeout, strings.ToLower(op), id.Name())
	if t.progress.Waiting != "" {
		message += fmt.Sprintf(", for the state %q", t.progress.Waiting)
	}
	if t.progress.State != "" {
		message += fmt.Sprintf(". AWS still reports %q", t.progress.State)
	}
	return message + ". Deploy again to continue."
}
