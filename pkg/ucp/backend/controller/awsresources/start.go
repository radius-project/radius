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
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"uuid"

	"github.com/wI2L/jsondiff"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_aws "github.com/radius-project/radius/pkg/ucp/resources/aws"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// sendStart sends the first call of a native operation that waits for AWS to take it, because the handler reported
// that something the resource depends on is busy (native.NotReady). It returns the record once AWS took the call, so
// Run waits for the operation as for any other, or nil and the operation's result.
//
// Until the handler's ReadyTimeout passes, counted from when this worker started, it:
//
//  1. Saves that it sends the call (records.StartSending), checking the ETag. If the record no longer shows this
//     operation, a new request replaced it, and the worker stops without calling AWS.
//  2. Sends the call with the tokens the request saved: the client token and generated name, the update token and
//     state, or the delete token.
//  3. When the handler reports NotReady again, saves that it waits (records.StartWaiting) and tries again later.
//
// Once AWS took the call, the record no longer waits. Any other error settles the record as a start call that failed
// at the request does (records.StartFailed). When the timeout passes, nothing reached AWS, so the operation fails and
// the record is settled as rejected.
//
// A worker that finds the call may have been in flight when an earlier run stopped can't tell whether AWS took it, so
// the operation fails with an unknown outcome, and the next request continues as after any interrupted call: it finds
// a create by the handler's Recovery, sends an update again with its token, and sends a delete again with its delete
// token. The worker never sends that call itself, because a call AWS may have must never go back to waiting, where a
// new request could replace it. A stop while waiting leaves the record waiting, and the redelivered operation waits
// again.
func (c *AwaitController) sendStart(ctx context.Context, store *records.Store, id resources.ID, request *ctrl.Request, op string) (*datamodel.AWSResource, ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	typeName := resources_aws.ToAWSResourceType(id)
	t := c.clients.Native.Lookup(typeName)
	rec, _, err := store.Get(ctx, id)
	if err != nil {
		return nil, requeue(ctx, err), nil
	} else if rec == nil || !sameRequest(rec.Properties.RequestToken, request.OperationID) {
		return nil, superseded(id), nil
	}
	if t == nil && records.IsWaiting(rec) {
		message := fmt.Sprintf("A native handler started the operation on the AWS resource %q, but UCP has no native handler for %s. Nothing was sent to AWS. Deploy again.", id.Name(), typeName)
		return nil, endWaiting(ctx, store, id, request, message), nil
	} else if t == nil {
		message := fmt.Sprintf("A native handler started the operation on the AWS resource %q, but UCP has no native handler for %s. Deploy again.", id.Name(), typeName)
		return nil, endStart(ctx, store, id, request, op, native.OutcomeUnknown, "", v1.ErrorDetails{Code: v1.CodeInternal, Message: message}), nil
	}
	if rec.Properties.StartState == records.StartSending {
		message := fmt.Sprintf("UCP stopped while it sent the %s of the AWS resource %q, so it can't tell whether AWS took it. Deploy again to continue.", strings.ToLower(op), id.Name())
		return nil, endStart(ctx, store, id, request, op, native.OutcomeUnknown, "", v1.ErrorDetails{Code: v1.CodeInternal, Message: message}), nil
	}

	region := id.FindScope(resources_aws.ScopeRegions)
	timeout := t.Handler.Options().ReadyTimeout
	deadline := now().Add(timeout)
	interval := ucp_aws.MinPollInterval
	for {
		if !now().Before(deadline) {
			message := fmt.Sprintf("UCP waited %s for AWS to take the %s of the AWS resource %q, because %s. Nothing was sent to AWS. Deploy again.", timeout, strings.ToLower(op), id.Name(), rec.Properties.StartReason)
			return nil, endWaiting(ctx, store, id, request, message), nil
		}

		rec, err = saveStart(ctx, store, id, request.OperationID, func(rec *datamodel.AWSResource) {
			rec.Properties.StartState = records.StartSending
		})
		if errors.Is(err, errSuperseded) {
			return nil, superseded(id), nil
		} else if err != nil {
			return nil, requeue(ctx, err), nil
		}

		logger.Info("Sending the first call of a native AWS operation", "alias", id.Name(), "operation", op)
		created, err := send(ctx, t, region, rec, op)
		var notReady *native.NotReady
		switch {
		case err != nil && !errors.As(err, &notReady) && ctx.Err() != nil:
			// The worker stopped, and the call may have reached AWS. The record still shows it in flight, so the
			// redelivered operation reads it as an interrupted call.
			return nil, ctrl.Result{}, fmt.Errorf("stopped sending the %s of the AWS resource %q: %w", strings.ToLower(op), id.Name(), ctx.Err())
		case err == nil:
			rec, err = saveStart(ctx, store, id, request.OperationID, func(rec *datamodel.AWSResource) {
				records.ClearStart(rec)
				if created != "" {
					rec.Properties.AWSIdentifier = created
				}
			})
			if errors.Is(err, errSuperseded) {
				return nil, superseded(id), nil
			} else if err != nil {
				// AWS took the call, and the record still shows it in flight, so the redelivered operation fails with
				// an unknown outcome, and the next deploy finds the operation in AWS.
				return nil, requeue(ctx, err), nil
			}
			return rec, ctrl.Result{}, nil
		case errors.As(err, &notReady):
			reason := notReady.Reason
			rec, err = saveStart(ctx, store, id, request.OperationID, func(rec *datamodel.AWSResource) {
				rec.Properties.StartState = records.StartWaiting
				rec.Properties.StartReason = reason
			})
			if errors.Is(err, errSuperseded) {
				return nil, superseded(id), nil
			} else if err != nil {
				return nil, requeue(ctx, err), nil
			}
		case op == records.OperationDelete && isGone(err):
			// Nothing of the type is left to delete, as when the request finds so.
			if err := update(ctx, store, id, request.OperationID, func(*datamodel.AWSResource) bool { return true }); errors.Is(err, errSuperseded) {
				return nil, superseded(id), nil
			} else if err != nil {
				return nil, requeue(ctx, err), nil
			}
			return nil, ctrl.Result{}, nil
		default:
			return nil, endStart(ctx, store, id, request, op, records.OutcomeOf(err), created, startError(err)), nil
		}

		select {
		case <-ctx.Done():
			return nil, ctrl.Result{}, fmt.Errorf("stopped waiting to send the %s of the AWS resource %q: %w", strings.ToLower(op), id.Name(), ctx.Err())
		case <-time.After(interval):
		}
		interval = min(interval*2, ucp_aws.MaxPollInterval)
	}
}

// send makes the first call of op with what the record saved for it. A deferred call carries no write-only value, so
// a create sends the pending properties, with the generated name, and an update gets the pending properties as
// Desired.
func send(ctx context.Context, t *native.Type, region string, rec *datamodel.AWSResource, op string) (string, error) {
	switch op {
	case records.OperationCreate:
		properties, err := t.WithName(maps.Clone(rec.Properties.PendingLastApplied), rec.Properties.ClientToken)
		if err != nil {
			return "", err
		}
		if properties == nil {
			properties = map[string]any{}
		}
		return t.Handler.Create(ctx, native.CreateRequest{Region: region, Properties: properties, ClientToken: rec.Properties.ClientToken})
	case records.OperationUpdate:
		// The request saved the patch from a jsondiff.Patch, so it always converts back.
		patch := jsondiff.Patch{}
		b, _ := json.Marshal(rec.Properties.StartPatch)
		_ = json.Unmarshal(b, &patch)
		return "", t.Handler.Update(ctx, native.UpdateRequest{
			Region: region, Identifier: rec.Properties.AWSIdentifier,
			Current: rec.Properties.StartCurrent, Desired: rec.Properties.PendingLastApplied, Patch: patch,
			Token: rec.Properties.UpdateToken, State: rec.Properties.UpdateState,
		})
	default:
		return "", t.StartDelete(ctx, region, rec.Properties.AWSIdentifier, nil, rec.Properties.DeleteToken)
	}
}

// saveStart applies change to the record of the operation operationID and saves it, checking the ETag, and returns
// the record as saved. It returns errSuperseded when the record no longer shows the operation, including when a new
// request removed it, even after an attempt that lost the race to it.
func saveStart(ctx context.Context, store *records.Store, id resources.ID, operationID uuid.UUID, change func(rec *datamodel.AWSResource)) (*datamodel.AWSResource, error) {
	saved, err := updateFound(ctx, store, id, operationID, func(rec *datamodel.AWSResource) bool {
		change(rec)
		return false
	})
	if err == nil && saved == nil {
		return nil, errSuperseded
	}
	return saved, err
}

// endStart fails the operation op, whose first call AWS never took or failed, with failure. It
// settles the record by outcome, as when the call fails at the request (records.StartFailed): a first create AWS
// rejected, and a delete AWS fully applied, remove the record. created is the identifier a failed create returned.
func endStart(ctx context.Context, store *records.Store, id resources.ID, request *ctrl.Request, op string, outcome native.Outcome, created string, failure v1.ErrorDetails) ctrl.Result {
	return endWith(ctx, store, id, request, failure, func(rec *datamodel.AWSResource) bool {
		return records.StartFailed(rec, op, outcome, created, false)
	})
}

// endWith fails the operation with failure, after settle sets the record's values. When settle returns true, the
// record is removed.
func endWith(ctx context.Context, store *records.Store, id resources.ID, request *ctrl.Request, failure v1.ErrorDetails, settle func(rec *datamodel.AWSResource) bool) ctrl.Result {
	failure.Target = request.ResourceID
	err := update(ctx, store, id, request.OperationID, func(rec *datamodel.AWSResource) bool {
		if settle(rec) {
			return true
		}
		records.FailStart(rec, &failure)
		return false
	})
	if errors.Is(err, errSuperseded) {
		return superseded(id)
	} else if err != nil {
		return requeue(ctx, err)
	}
	return ctrl.NewFailedResult(failure)
}

// startError returns the error details of a first call AWS refused: invalid for a client failure, such as an invalid
// property value, as at the request, and internal otherwise.
func startError(err error) v1.ErrorDetails {
	code := v1.CodeInternal
	var failure *native.Failure
	if (errors.As(err, &failure) && failure.ClientError) || ucp_aws.IsClientFault(err) {
		code = v1.CodeInvalid
	}
	return v1.ErrorDetails{Code: code, Message: err.Error()}
}

// endWaiting fails the operation, which waits and sent nothing to AWS, with message, and settles the record as if AWS
// rejected the call (records.DropWaiting): a first create removes the record.
func endWaiting(ctx context.Context, store *records.Store, id resources.ID, request *ctrl.Request, message string) ctrl.Result {
	return endWith(ctx, store, id, request, v1.ErrorDetails{Code: v1.CodeInternal, Message: message}, records.DropWaiting)
}

// isGone reports whether err means a delete has nothing of its type to delete: AWS has no such resource, or the
// identifier now names a resource of another product.
func isGone(err error) bool {
	var other *native.OtherProductError
	return errors.Is(err, native.ErrNotFound) || errors.As(err, &other)
}
