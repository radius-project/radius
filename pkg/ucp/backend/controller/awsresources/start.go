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
//  1. Saves that it sends the call (records.StartSending), checking the ETag, only while the record still shows the
//     operation waiting. If the record no longer shows this operation, a new request replaced it, and the worker stops
//     without calling AWS. If another delivery of the operation saved that it sends the call, this one can't tell
//     whether AWS took it, so the operation fails with an unknown outcome, and the next request recovers it. Only the
//     delivery that saved it sends the call and saves what came of it.
//  2. Sends the call with the tokens the request saved: the client token and generated name, the update token and
//     state, or the delete token.
//  3. When the handler reports NotReady again, saves that it waits (records.StartWaiting) and tries again later.
//
// Once AWS took the call, the record no longer waits. Any other error settles the record as a start call that failed
// at the request does (records.StartFailed). When the timeout passes, nothing reached AWS, so the operation fails and
// the record is settled as rejected. A first create AWS never had keeps a record that holds no resource, instead of
// removing it, so a redelivered operation reports the same failure (see endWith).
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
	if t == nil {
		message := fmt.Sprintf("A native handler started the operation on the AWS resource %q, but UCP has no native handler for %s. Deploy again.", id.Name(), typeName)
		result, claimed := endWaiting(ctx, store, id, request, message)
		if claimed != nil {
			result = endStart(ctx, store, id, request, op, native.OutcomeUnknown, "", v1.ErrorDetails{Code: v1.CodeInternal, Message: message})
		}
		return nil, result, nil
	}

	region := id.FindScope(resources_aws.ScopeRegions)
	timeout := t.Handler.Options().ReadyTimeout
	deadline := now().Add(timeout)
	interval := ucp_aws.MinPollInterval
	for {
		var rec *datamodel.AWSResource
		var claimed *claimTaken
		var err error
		if !now().Before(deadline) {
			reason := ""
			if current, _, getErr := store.Get(ctx, id); getErr == nil && current != nil {
				reason = current.Properties.StartReason
			}
			message := fmt.Sprintf("UCP waited %s for AWS to take the %s of the AWS resource %q, because %s. Nothing was sent to AWS. Deploy again.", timeout, strings.ToLower(op), id.Name(), reason)
			var result ctrl.Result
			if result, claimed = endWaiting(ctx, store, id, request, message); claimed == nil {
				return nil, result, nil
			}
		} else {
			rec, err = saveStart(ctx, store, id, request.OperationID, records.StartWaiting, func(rec *datamodel.AWSResource) {
				sent := records.Now().UTC()
				rec.Properties.StartState = records.StartSending
				rec.Properties.StartSentAt = &sent
			})
			errors.As(err, &claimed)
		}
		switch {
		case claimed != nil && claimed.state == "":
			// Another delivery of this operation sent the call, and AWS took it, so this one only waits for it.
			return claimed.rec, ctrl.Result{}, nil
		case claimed != nil && records.IsSendingNow(claimed.rec):
			// Another delivery of this operation sends the call now. Releasing the operation would let a request
			// remove the record while that call creates the resource, so this one waits until the claim ends.
			if err := sleep(ctx, interval); err != nil {
				return nil, ctrl.Result{}, fmt.Errorf("stopped waiting for another attempt to send the %s of the AWS resource %q: %w", strings.ToLower(op), id.Name(), err)
			}
			interval = min(interval*2, ucp_aws.MaxPollInterval)
			continue
		case claimed != nil:
			message := fmt.Sprintf("UCP stopped while it sent the %s of the AWS resource %q, so it can't tell whether AWS took it. Deploy again to continue.", strings.ToLower(op), id.Name())
			return nil, endStart(ctx, store, id, request, op, native.OutcomeUnknown, "", v1.ErrorDetails{Code: v1.CodeInternal, Message: message}), nil
		case errors.Is(err, errSuperseded):
			return nil, supersededOrFinished(ctx, store, id, request), nil
		case err != nil:
			return nil, requeue(ctx, err), nil
		}

		logger.Info("Sending the first call of a native AWS operation", "alias", id.Name(), "operation", op)
		created, err := send(ctx, t, region, rec, op)
		var notReady *native.NotReady
		switch {
		case err != nil && !errors.As(err, &notReady) && ctx.Err() != nil:
			// The worker stopped, and the call may have reached AWS. The record still shows it in flight, so the
			// redelivered operation reads it as an interrupted call once the claim is old.
			return nil, ctrl.Result{}, fmt.Errorf("stopped sending the %s of the AWS resource %q: %w", strings.ToLower(op), id.Name(), ctx.Err())
		case err == nil:
			rec, err = saveStart(ctx, store, id, request.OperationID, records.StartSending, func(rec *datamodel.AWSResource) {
				records.ClearStart(rec)
				if created != "" {
					rec.Properties.AWSIdentifier = created
				}
			})
			if errors.Is(err, errSuperseded) {
				return nil, supersededOrFinished(ctx, store, id, request), nil
			} else if err != nil {
				// AWS took the call, and the record still shows it in flight, so the redelivered operation fails with
				// an unknown outcome once the claim is old, and the next deploy finds the operation in AWS.
				return nil, requeue(ctx, err), nil
			}
			return rec, ctrl.Result{}, nil
		case errors.As(err, &notReady):
			reason := notReady.Reason
			_, err = saveStart(ctx, store, id, request.OperationID, records.StartSending, func(rec *datamodel.AWSResource) {
				rec.Properties.StartState = records.StartWaiting
				rec.Properties.StartReason = reason
				rec.Properties.StartSentAt = nil
			})
			if errors.Is(err, errSuperseded) {
				return nil, supersededOrFinished(ctx, store, id, request), nil
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

		if err := sleep(ctx, interval); err != nil {
			return nil, ctrl.Result{}, fmt.Errorf("stopped waiting to send the %s of the AWS resource %q: %w", strings.ToLower(op), id.Name(), err)
		}
		interval = min(interval*2, ucp_aws.MaxPollInterval)
	}
}

// sleep waits for interval, or returns ctx's error when ctx ends first.
func sleep(ctx context.Context, interval time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(interval):
		return nil
	}
}

// supersededOrFinished returns the result of an operation whose record no longer shows it: the result saved on the
// record when another delivery of the operation already finished it, and otherwise canceled, because a newer
// operation replaced it.
func supersededOrFinished(ctx context.Context, store *records.Store, id resources.ID, request *ctrl.Request) ctrl.Result {
	rec, _, err := store.Get(ctx, id)
	if err == nil && rec != nil && sameRequest(rec.Properties.LastRequestToken, request.OperationID) {
		return finishedResult(rec)
	}
	return superseded(id)
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
// the record as saved. Only a record whose start state is from changes, so only one delivery of the operation, the one
// that saved StartSending, sends the call and saves what came of it. It returns a *claimTaken when the start state is
// another, and errSuperseded when the record no longer shows the operation, including when a new request removed it,
// even after an attempt that lost the race to it.
func saveStart(ctx context.Context, store *records.Store, id resources.ID, operationID uuid.UUID, from string, change func(rec *datamodel.AWSResource)) (*datamodel.AWSResource, error) {
	var taken *claimTaken
	saved, err := updateFound(ctx, store, id, operationID, func(rec *datamodel.AWSResource) bool {
		// A save that lost a race runs this again on the record as it is now.
		taken = nil
		if rec.Properties.StartState != from {
			taken = &claimTaken{state: rec.Properties.StartState, rec: rec}
			return false
		}
		change(rec)
		return false
	})
	switch {
	case taken != nil && err == nil:
		return nil, taken
	case err == nil && saved == nil:
		return nil, errSuperseded
	}
	return saved, err
}

// claimTaken means the start state of the record isn't the one a delivery expected, because another delivery of the
// operation changed it: state is the start state the record has, and rec is the record.
type claimTaken struct {
	state string
	rec   *datamodel.AWSResource
}

func (c *claimTaken) Error() string {
	return fmt.Sprintf("another delivery of the operation changed its start state to %q", c.state)
}

// endStart fails the operation op, whose first call AWS never took or failed, with failure. It settles the record by
// outcome, as when the call fails at the request (records.StartFailed), and then as endWith says. created is the
// identifier a failed create returned.
func endStart(ctx context.Context, store *records.Store, id resources.ID, request *ctrl.Request, op string, outcome native.Outcome, created string, failure v1.ErrorDetails) ctrl.Result {
	return endWith(ctx, store, id, request, failure, func(rec *datamodel.AWSResource) bool {
		return records.StartFailed(rec, op, outcome, created, false)
	})
}

// endWith fails the operation with failure, after settle sets the record's values. When settle returns true, AWS has
// nothing of the operation's resource: a delete AWS fully applied removes the record, and a first create AWS never had
// keeps a record that holds no resource (records.DropCreate), so a redelivered operation reports failure again.
func endWith(ctx context.Context, store *records.Store, id resources.ID, request *ctrl.Request, failure v1.ErrorDetails, settle func(rec *datamodel.AWSResource) bool) ctrl.Result {
	failure.Target = request.ResourceID
	err := update(ctx, store, id, request.OperationID, func(rec *datamodel.AWSResource) bool {
		op := records.PendingOperation(rec)
		if settle(rec) {
			if op == records.OperationDelete {
				return true
			}
			records.DropCreate(rec)
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
// rejected the call (records.DropWaiting): a first create keeps a record that holds no resource (see endWith). It
// changes the record only while it still shows the operation waiting. Otherwise it returns a *claimTaken, because
// another delivery of the operation sends the call, or AWS took it, and the caller decides.
func endWaiting(ctx context.Context, store *records.Store, id resources.ID, request *ctrl.Request, message string) (ctrl.Result, *claimTaken) {
	failure := v1.ErrorDetails{Code: v1.CodeInternal, Message: message, Target: request.ResourceID}
	_, err := saveStart(ctx, store, id, request.OperationID, records.StartWaiting, func(rec *datamodel.AWSResource) {
		if records.DropWaiting(rec) {
			records.DropCreate(rec)
		}
		records.FailStart(rec, &failure)
	})
	var claimed *claimTaken
	switch {
	case errors.As(err, &claimed):
		return ctrl.Result{}, claimed
	case errors.Is(err, errSuperseded):
		return supersededOrFinished(ctx, store, id, request), nil
	case err != nil:
		return requeue(ctx, err), nil
	}
	return ctrl.NewFailedResult(failure), nil
}

// isGone reports whether err means a delete has nothing of its type to delete: AWS has no such resource, or the
// identifier now names a resource of another product.
func isGone(err error) bool {
	var other *native.OtherProductError
	return errors.Is(err, native.ErrNotFound) || errors.As(err, &other)
}
