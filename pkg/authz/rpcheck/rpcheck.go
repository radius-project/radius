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

// Package rpcheck verifies, in a resource provider, the operation UCP assigned to a
// write (B5) and claims it in the async worker (B6).
package rpcheck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/radius-project/radius/pkg/authz/records"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var (
	selfOnce sync.Once
	selfName string
	selfErr  error
)

// Self returns this component's name from its certificate.
func Self() (string, error) {
	selfOnce.Do(func() {
		id, err := mtls.LocalIdentity(authz.ProcessOptions())
		selfName, selfErr = id.Component, err
	})
	return selfName, selfErr
}

// SetSelfForTest overrides the component name returned by Self.
func SetSelfForTest(name string) {
	selfOnce.Do(func() {})
	selfName, selfErr = name, nil
}

// Middleware verifies that each write names an open operation assigned to this
// component under an active record, for this resource and action, with a matching body.
// It must run after the ARM request context middleware.
func Middleware(db database.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mode := authz.ProcessMode()
			if mode == authz.ModeOff || !records.IsWrite(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			ctx := r.Context()
			armCtx := v1.ARMRequestContextFromContext(ctx)
			if armCtx == nil || armCtx.ResourceID.IsEmpty() {
				next.ServeHTTP(w, r)
				return
			}
			target := armCtx.ResourceID.String()
			action := records.ActionFor(r.Method, armCtx.ResourceID.Type())

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read request body", http.StatusBadRequest)
				return
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))

			ids := records.IDs{RecordID: r.Header.Get(records.HeaderRecord), OperationID: r.Header.Get(records.HeaderOperation)}
			code, err := CheckOperation(ctx, records.NewStore(db), ids.OperationID, target, action, body)
			if err != nil {
				code = v1.CodeAuthorizationUnavailable
				ucplog.FromContextOrDiscard(ctx).Error(err, "failed to check authorization operation")
			}
			if code != "" {
				if err := authz.Apply(ctx, mode, authz.Decision{Code: code, Action: action, Target: target}); err != nil {
					authz.WriteDenied(ctx, w, r, err)
					return
				}
				// dryRun: the write continues. A missing or unusable operation is replaced by an
				// unbound one so the data save and async claim don't log the same denial again.
				if code != v1.CodeOperationInputMismatch {
					unbound, err := unboundOperation(ctx, records.NewStore(db), target, action, body)
					if err != nil {
						ucplog.FromContextOrDiscard(ctx).Error(err, "failed to create unbound dryRun operation")
					} else {
						ids = unbound
					}
				}
			}
			next.ServeHTTP(w, r.WithContext(records.WithIDs(ctx, ids)))
		})
	}
}

// unboundOperation creates a record and an open operation assigned to this component for
// a dryRun write that failed its check.
func unboundOperation(ctx context.Context, store *records.Store, target, action string, body []byte) (records.IDs, error) {
	self, err := Self()
	if err != nil {
		return records.IDs{}, err
	}
	rec := store.NewRecord(records.Record{Subject: "unbound:" + self, Submitter: self, Actions: []string{action}, Targets: []string{target}})
	if _, err := store.SaveRecord(ctx, rec, ""); err != nil {
		return records.IDs{}, err
	}
	op := &records.Operation{RecordID: rec.ID, ResourceID: target, Action: action, AssignedComponent: self, InputHash: records.InputHash(body)}
	if err := store.CreateOperation(ctx, op); err != nil {
		return records.IDs{}, err
	}
	return records.IDs{RecordID: rec.ID, OperationID: op.ID}, nil
}

// CheckOperation returns the denial code for a write under operationID, or "".
func CheckOperation(ctx context.Context, store *records.Store, operationID, target, action string, body []byte) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	self, err := Self()
	if err != nil {
		return "", err
	}
	if operationID == "" {
		logger.Info("write has no authorization operation")
		return v1.CodeOperationNotAssigned, nil
	}
	op, _, err := store.GetOperation(ctx, operationID)
	if errors.Is(err, &database.ErrNotFound{}) {
		return v1.CodeOperationNotAssigned, nil
	} else if err != nil {
		return "", err
	}
	if op.AssignedComponent != self || !op.Open() {
		logger.Info("operation is not open or not assigned to this component", "authzOperation", op.ID, "authzAssignedComponent", op.AssignedComponent, "authzOperationStatus", op.Status)
		return v1.CodeOperationNotAssigned, nil
	}
	rec, _, err := store.GetRecord(ctx, op.RecordID)
	if errors.Is(err, &database.ErrNotFound{}) {
		return v1.CodeExecutionRecordNotActive, nil
	} else if err != nil {
		return "", err
	}
	if !rec.Active(store.CurrentTime()) {
		return v1.CodeExecutionRecordNotActive, nil
	}
	if !records.SameResource(op.ResourceID, target) || (op.Action != "" && !records.SameResource(op.Action, action)) {
		logger.Info("write does not match the operation", "authzOperation", op.ID, "authzOperationResource", op.ResourceID, "authzOperationAction", op.Action)
		return v1.CodeGrantScopeExceeded, nil
	}
	if op.InputHash != records.InputHash(body) {
		logger.Info("write body does not match the operation input hash", "authzOperation", op.ID)
		return v1.CodeOperationInputMismatch, nil
	}
	return "", nil
}

// Claim claims the operation named by an async request for this worker (B6). It
// returns a denial code, or "" when the worker may run the operation.
func Claim(ctx context.Context, db database.Client, operationID string) (string, error) {
	self, err := Self()
	if err != nil {
		return "", err
	}
	// UCP is the authority: its own async work (for example tracked resources) was
	// authorized when UCP accepted the request.
	if self == "ucp" && operationID == "" {
		return "", nil
	}
	if operationID == "" {
		return v1.CodeOperationNotAssigned, nil
	}
	store := records.NewStore(db)
	op, err := store.Claim(ctx, operationID, self)
	switch {
	case errors.Is(err, records.ErrClaimed):
		return v1.CodeOperationNotAssigned, nil
	case errors.Is(err, &database.ErrNotFound{}):
		return v1.CodeOperationNotAssigned, nil
	case err != nil:
		return "", fmt.Errorf("failed to claim authorization operation: %w", err)
	}
	if op.AssignedComponent != self || op.Status != records.OperationRunning || op.LeaseHolder != self {
		return v1.CodeOperationNotAssigned, nil
	}
	rec, _, err := store.GetRecord(ctx, op.RecordID)
	if err != nil || !rec.Active(store.CurrentTime()) {
		return v1.CodeExecutionRecordNotActive, nil
	}
	return "", nil
}

// Finish moves the operation to its final status. It is a no-op without an operation.
func Finish(ctx context.Context, db database.Client, operationID string, succeeded bool) {
	if operationID == "" || authz.ProcessMode() == authz.ModeOff {
		return
	}
	status := records.OperationSucceeded
	if !succeeded {
		status = records.OperationFailed
	}
	if err := records.NewStore(db).FinishOperation(ctx, operationID, status); err != nil {
		ucplog.FromContextOrDiscard(ctx).Error(err, "failed to finish authorization operation", "authzOperation", operationID)
	}
}
