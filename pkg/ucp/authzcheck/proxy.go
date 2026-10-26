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

package authzcheck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/records"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// LogFieldRecord and LogFieldOperation are the log keys for the record and operation.
	LogFieldRecord    = "authzRecord"
	LogFieldOperation = "authzOperation"

	deploymentType = "Microsoft.Resources/deployments"
)

// Grant is the record and operation UCP assigned to a proxied write.
type Grant struct {
	RecordID    string
	OperationID string
	store       *records.Store
}

// SetHeaders adds the record and operation headers to the downstream request.
func (g *Grant) SetHeaders(req *http.Request) {
	if g == nil {
		return
	}
	req.Header.Set(records.HeaderRecord, g.RecordID)
	req.Header.Set(records.HeaderOperation, g.OperationID)
}

// Finish records the result of a synchronous downstream response. Async responses stay
// open: the RP's async worker finishes them.
func (g *Grant) Finish(ctx context.Context, resp *http.Response) {
	if g == nil || resp == nil {
		return
	}
	async := resp.Header.Get("Azure-AsyncOperation") != "" || resp.Header.Get("Location") != ""
	if (resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted) && async {
		return
	}
	status := records.OperationSucceeded
	if resp.StatusCode >= 300 {
		status = records.OperationFailed
	}
	if err := g.store.FinishOperation(ctx, g.OperationID, status); err != nil {
		ucplog.FromContextOrDiscard(ctx).Error(err, "failed to finish authorization operation", LogFieldOperation, g.OperationID)
	}
}

// AuthorizeProxyWrite checks a write that UCP is about to proxy to a resource provider
// and creates the operation the provider must verify (B2-B4, C2). Reads return a nil
// Grant. A non-nil error is a *authz.DeniedError in enforce mode, or a failure.
//
//   - A user write creates a record for the user. A deployment's record targets the
//     deployment's resource group and names bicep-de as the submitter.
//   - A controller write must name a mapped namespace, and the target must be in the
//     namespace's resource group. It then gets a record like a user write.
//   - Other component writes must name an active record that covers the target. The
//     caller must be the record's submitter or the component assigned to the named open
//     operation. A component's deployment write creates a child record that must be a
//     subset of its parent.
//   - PROTOTYPE SHORTCUT: the engine cannot send record IDs yet (B7), so engine writes
//     without a record header bind to the newest active bicep-de record covering the target.
func AuthorizeProxyWrite(ctx context.Context, db database.Client, req *http.Request, id resources.ID, downstream *url.URL) (*Grant, error) {
	mode := authz.ProcessMode()
	if mode == authz.ModeOff || !records.IsWrite(req.Method) {
		return nil, nil
	}

	body, err := readBody(req)
	if err != nil {
		return nil, err
	}

	store := records.NewStore(db)
	caller, ok := CallerFromContext(ctx)
	if !ok {
		caller, _ = CallerFromContext(req.Context())
	}
	action := records.ActionFor(req.Method, id.Type())
	target := id.String()
	isDeploymentPut := strings.EqualFold(id.Type(), deploymentType) && req.Method == http.MethodPut

	rec, code, err := bindRecord(ctx, store, req, caller, id, action, isDeploymentPut)
	if err != nil {
		return nil, err
	}

	if code != "" {
		if err := authz.Apply(ctx, mode, authz.Decision{Code: code, Action: action, Target: target}); err != nil {
			return nil, err
		}
		// dryRun: continue under an unbound record so downstream checks see a consistent operation.
		rec = store.NewRecord(records.Record{Subject: "unbound:" + caller.String(), Submitter: caller.Name, Actions: []string{action}, Targets: []string{target}})
		if _, err := store.SaveRecord(ctx, rec, ""); err != nil {
			return nil, err
		}
	}

	op := &records.Operation{
		RecordID:          rec.ID,
		ResourceID:        target,
		Action:            action,
		AssignedComponent: componentForURL(downstream),
		InputHash:         records.InputHash(body),
	}
	if err := store.CreateOperation(ctx, op); err != nil {
		return nil, fmt.Errorf("failed to create authorization operation: %w", err)
	}

	ucplog.FromContextOrDiscard(ctx).V(ucplog.LevelDebug).Info("authorized proxied write",
		LogFieldRecord, rec.ID, LogFieldOperation, op.ID, "authzAssignedComponent", op.AssignedComponent)
	return &Grant{RecordID: rec.ID, OperationID: op.ID, store: store}, nil
}

// bindRecord returns the record the write runs under, or a denial code.
func bindRecord(ctx context.Context, store *records.Store, req *http.Request, caller Caller, id resources.ID, action string, isDeploymentPut bool) (*records.Record, string, error) {
	target := id.String()
	options := authz.ProcessOptions()

	switch {
	case caller.Kind == CallerUser:
		rec, err := newSubjectRecord(ctx, store, "user:"+caller.Name, id, action, isDeploymentPut)
		return rec, "", err

	case caller.Kind == CallerComponent && caller.Name == ComponentController:
		ns := req.Header.Get(records.HeaderSourceNamespace)
		mapping, ok := options.MappingFor(ns)
		if ns == "" || !ok {
			ucplog.FromContextOrDiscard(ctx).Info("controller write has no namespace mapping", "authzSourceNamespace", ns)
			return nil, v1.CodeAuthorizationFailed, nil
		}
		if !records.ScopeCovers(mapping.ResourceGroup, target) {
			ucplog.FromContextOrDiscard(ctx).Info("controller write is outside its namespace mapping", "authzSourceNamespace", ns, "authzMappedResourceGroup", mapping.ResourceGroup)
			return nil, v1.CodeGrantScopeExceeded, nil
		}
		rec, err := newSubjectRecord(ctx, store, "namespace:"+ns, id, action, isDeploymentPut)
		return rec, "", err

	case caller.Kind == CallerComponent:
		parent, code, err := bindComponentRecord(ctx, store, req, caller, action, target)
		if err != nil || code != "" {
			return nil, code, err
		}
		if !isDeploymentPut {
			return parent, "", nil
		}
		// B4: a nested deployment gets a child record that must be a subset of its parent.
		child := store.NewRecord(records.Record{
			Subject:    parent.Subject,
			Submitter:  ComponentDeploymentEngine,
			Actions:    []string{records.ActionAll},
			Targets:    []string{records.ResourceGroupScope(id)},
			ParentID:   parent.ID,
			Deployment: target,
			ExpiresAt:  parent.ExpiresAt,
		})
		if !child.IsSubsetOf(parent) {
			return nil, v1.CodeGrantScopeExceeded, nil
		}
		if _, err := store.SaveRecord(ctx, child, ""); err != nil {
			return nil, "", err
		}
		return child, "", nil

	default:
		return nil, v1.CodeAuthorizationFailed, nil
	}
}

// newSubjectRecord creates the record for a write requested by a user or a mapped namespace.
func newSubjectRecord(ctx context.Context, store *records.Store, subject string, id resources.ID, action string, isDeploymentPut bool) (*records.Record, error) {
	r := records.Record{Subject: subject, Submitter: "ucp", Actions: []string{records.ActionAll}, Targets: []string{records.ResourceGroupScope(id)}}
	if approval := records.UserApprovalFromContext(ctx); approval != nil {
		r.UserApproval = approval
		r.Actions, r.Targets = nil, nil
		for _, approved := range approval.Resources {
			r.Actions = append(r.Actions, approved.Action)
			r.Targets = append(r.Targets, approved.ResourceID)
		}
	}
	if isDeploymentPut {
		r.Submitter = ComponentDeploymentEngine
		r.Deployment = id.String()
		// A new deployment supersedes the previous record for the same deployment.
		if active, err := store.ActiveRecords(ctx, ComponentDeploymentEngine); err == nil {
			for _, old := range active {
				if old.ParentID == "" && records.SameResource(old.Deployment, id.String()) {
					_ = store.CloseRecord(ctx, old.ID, records.RecordClosed)
				}
			}
		}
	}
	rec := store.NewRecord(r)
	if _, err := store.SaveRecord(ctx, rec, ""); err != nil {
		return nil, err
	}
	return rec, nil
}

// bindComponentRecord finds the active record a component write runs under.
func bindComponentRecord(ctx context.Context, store *records.Store, req *http.Request, caller Caller, action, target string) (*records.Record, string, error) {
	recordID := req.Header.Get(records.HeaderRecord)
	if recordID == "" {
		if caller.Name != ComponentDeploymentEngine {
			return nil, v1.CodeExecutionRecordNotActive, nil
		}
		return bindEngineRecord(ctx, store, action, target)
	}

	rec, _, err := store.GetRecord(ctx, recordID)
	if errors.Is(err, &database.ErrNotFound{}) {
		return nil, v1.CodeExecutionRecordNotActive, nil
	} else if err != nil {
		return nil, "", err
	}
	if !rec.Active(store.CurrentTime()) {
		return nil, v1.CodeExecutionRecordNotActive, nil
	}
	if !rec.Covers(action, target) {
		return nil, v1.CodeGrantScopeExceeded, nil
	}
	if rec.Submitter == caller.Name {
		return rec, "", nil
	}

	// A component working on an operation under this record may make follow-up writes.
	if opID := req.Header.Get(records.HeaderOperation); opID != "" {
		op, _, err := store.GetOperation(ctx, opID)
		if err == nil && records.SameResource(op.RecordID, rec.ID) && op.AssignedComponent == caller.Name && op.Open() {
			return rec, "", nil
		}
	}
	return nil, v1.CodeAuthorizationFailed, nil
}

// bindEngineRecord is the PROTOTYPE SHORTCUT for engine calls that carry no record ID.
func bindEngineRecord(ctx context.Context, store *records.Store, action, target string) (*records.Record, string, error) {
	active, err := store.ActiveRecords(ctx, ComponentDeploymentEngine)
	if err != nil {
		return nil, "", err
	}
	if len(active) == 0 {
		return nil, v1.CodeExecutionRecordNotActive, nil
	}
	var best *records.Record
	for _, r := range active {
		if r.Covers(action, target) && (best == nil || r.CreatedAt.After(best.CreatedAt)) {
			best = r
		}
	}
	if best == nil {
		return nil, v1.CodeGrantScopeExceeded, nil
	}
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.V(ucplog.LevelDebug).Info("engine write bound to active record by target (prototype shortcut until B7)",
		LogFieldPrototypeExemption, "engine-record-binding", LogFieldRecord, best.ID)
	return best, "", nil
}

func readBody(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	return body, nil
}

// componentForURL returns the component name of a downstream URL, such as
// "applications-rp" for http://applications-rp.radius-system:5443.
func componentForURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	host := u.Hostname()
	if i := strings.Index(host, "."); i >= 0 {
		host = host[:i]
	}
	return host
}
