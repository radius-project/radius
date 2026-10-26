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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/records"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// DataSavePath and DataDeletePath are the UCP data-access endpoints (D1), relative to the path base.
	DataSavePath   = "/authz/data/save"
	DataDeletePath = "/authz/data/delete"

	// recordsPath is the prototype test hook for execution records, relative to the path base.
	recordsPath = records.RootScope + "/providers/" + records.RecordResourceType
)

// DataSaveRequest is the body of a data-access save.
type DataSaveRequest struct {
	ID   string `json:"id"`
	ETag string `json:"etag,omitempty"`
	Data any    `json:"data"`
}

// DataDeleteRequest is the body of a data-access delete.
type DataDeleteRequest struct {
	ID   string `json:"id"`
	ETag string `json:"etag,omitempty"`
}

// DataResponse is the body of a successful data-access call.
type DataResponse struct {
	ETag string `json:"etag,omitempty"`
}

// Endpoints serves the UCP authorization endpoints that are not ARM resources: the
// data-access endpoint (D1) and the prototype records test hook.
type Endpoints struct {
	PathBase string
	DB       func(ctx context.Context) (database.Client, error)
}

// Middleware routes authorization endpoint requests and passes everything else to next.
// It must run inside the authentication middleware.
func (e *Endpoints) Middleware(next http.Handler) http.Handler {
	base := strings.TrimSuffix(strings.ToLower(e.PathBase), "/")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authz.ProcessMode() == authz.ModeOff {
			next.ServeHTTP(w, r)
			return
		}
		path := strings.ToLower(r.URL.Path)
		switch {
		case path == base+DataSavePath || path == base+DataDeletePath:
			e.serveData(w, r, path == base+DataSavePath)
		case strings.HasPrefix(path, base+strings.ToLower(recordsPath)):
			e.serveRecords(w, r, strings.TrimPrefix(r.URL.Path[len(base):], recordsPath))
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func (e *Endpoints) serveData(w http.ResponseWriter, r *http.Request, save bool) {
	ctx := r.Context()
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorBody("MethodNotAllowed", "use POST"))
		return
	}
	db, err := e.DB(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody("Internal", err.Error()))
		return
	}

	var id string
	var saveReq DataSaveRequest
	var deleteReq DataDeleteRequest
	if save {
		err = json.NewDecoder(r.Body).Decode(&saveReq)
		id = saveReq.ID
	} else {
		err = json.NewDecoder(r.Body).Decode(&deleteReq)
		id = deleteReq.ID
	}
	if err != nil || id == "" {
		writeJSON(w, http.StatusBadRequest, errorBody("BadRequest", "the request body must name a resource id"))
		return
	}

	action := "write"
	if !save {
		action = "delete"
	}
	caller, _ := CallerFromContext(ctx)
	code, err := CheckDataAccess(ctx, records.NewStore(db), caller, r.Header.Get(records.HeaderOperation), id)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorBody(v1.CodeAuthorizationUnavailable, err.Error()))
		return
	}
	if code != "" {
		if err := authz.Apply(ctx, authz.ProcessMode(), authz.Decision{Code: code, Action: action, Target: id}); err != nil {
			authz.WriteDenied(ctx, w, r, err)
			return
		}
	}

	if save {
		obj := &database.Object{Metadata: database.Metadata{ID: id}, Data: saveReq.Data}
		var opts []database.SaveOptions
		if saveReq.ETag != "" {
			opts = append(opts, database.WithETag(saveReq.ETag))
		}
		err = db.Save(ctx, obj, opts...)
		if err == nil {
			writeJSON(w, http.StatusOK, DataResponse{ETag: obj.ETag})
			return
		}
	} else {
		var opts []database.DeleteOptions
		if deleteReq.ETag != "" {
			opts = append(opts, database.WithETag(deleteReq.ETag))
		}
		err = db.Delete(ctx, id, opts...)
		if err == nil {
			writeJSON(w, http.StatusOK, DataResponse{})
			return
		}
	}
	writeDataError(w, err)
}

// CheckDataAccess checks a component's write to the database (D1). It returns a denial
// code, or "" when the write is allowed. The write must be made by the component
// assigned to an open operation, under an active record, for the operation's resource or
// an operation status. A running operation must be leased by the caller.
func CheckDataAccess(ctx context.Context, store *records.Store, caller Caller, operationID, id string) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	if caller.Kind != CallerComponent || operationID == "" {
		logger.Info("data write has no operation", LogFieldCallerName, caller.String())
		return v1.CodeOperationNotAssigned, nil
	}
	op, _, err := store.GetOperation(ctx, operationID)
	if errors.Is(err, &database.ErrNotFound{}) {
		return v1.CodeOperationNotAssigned, nil
	} else if err != nil {
		return "", err
	}
	if op.AssignedComponent != caller.Name {
		logger.Info("data write by a component the operation is not assigned to", LogFieldOperation, op.ID, "authzAssignedComponent", op.AssignedComponent)
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
	if !records.SameResource(op.ResourceID, id) && !isOperationStatus(id) {
		logger.Info("data write outside the operation's resource", LogFieldOperation, op.ID, "authzOperationResource", op.ResourceID)
		return v1.CodeGrantScopeExceeded, nil
	}
	if !op.Open() {
		return v1.CodeOperationNotAssigned, nil
	}
	if op.Status == records.OperationRunning && op.LeaseHolder != "" && op.LeaseHolder != caller.Name {
		return v1.CodeOperationNotAssigned, nil
	}
	return "", nil
}

func isOperationStatus(id string) bool {
	l := strings.ToLower(id)
	return strings.Contains(l, "/operationstatuses/") || strings.Contains(l, "/operationresults/")
}

// recordRequest is the body of a test hook PUT.
type recordRequest struct {
	Submitter  string   `json:"submitter"`
	Actions    []string `json:"actions"`
	Targets    []string `json:"targets"`
	TTL        string   `json:"ttl,omitempty"`
	Operations []struct {
		ResourceID        string          `json:"resourceId"`
		Action            string          `json:"action"`
		AssignedComponent string          `json:"assignedComponent"`
		Body              json.RawMessage `json:"body,omitempty"`
	} `json:"operations,omitempty"`
}

type recordResponse struct {
	*records.Record
	Operations []*records.Operation `json:"operations,omitempty"`
}

// serveRecords is the PROTOTYPE test hook for execution records. Users may list, read,
// create, and close records. Created records may include operations, so testers can
// call providers the way UCP would.
func (e *Endpoints) serveRecords(w http.ResponseWriter, r *http.Request, rest string) {
	ctx := r.Context()
	caller, _ := CallerFromContext(ctx)
	if caller.Kind != CallerUser {
		err := authz.Apply(ctx, authz.ModeEnforce, authz.Decision{Code: v1.CodeAuthorizationFailed, Action: r.Method, Target: r.URL.Path})
		authz.WriteDenied(ctx, w, r, err)
		return
	}
	db, err := e.DB(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody("Internal", err.Error()))
		return
	}
	store := records.NewStore(db)
	name := strings.Trim(rest, "/")

	switch {
	case r.Method == http.MethodGet && name == "":
		all, err := store.ListRecords(ctx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorBody("Internal", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"value": all})

	case r.Method == http.MethodGet:
		rec, _, err := store.GetRecord(ctx, records.RecordID(name))
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, rec)

	case r.Method == http.MethodDelete && name != "":
		if err := store.CloseRecord(ctx, records.RecordID(name), records.RecordRevoked); err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": string(records.RecordRevoked)})

	case r.Method == http.MethodPut && name != "":
		var body recordRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody("BadRequest", err.Error()))
			return
		}
		rec := store.NewRecord(records.Record{
			Subject:   "user:" + caller.Name,
			Submitter: body.Submitter,
			Actions:   body.Actions,
			Targets:   body.Targets,
		})
		rec.ID = records.RecordID(name)
		if body.TTL != "" {
			ttl, err := time.ParseDuration(body.TTL)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorBody("BadRequest", err.Error()))
				return
			}
			rec.ExpiresAt = rec.CreatedAt.Add(ttl)
		}
		if _, err := store.SaveRecord(ctx, rec, ""); err != nil {
			writeDataError(w, err)
			return
		}
		resp := recordResponse{Record: rec}
		for _, o := range body.Operations {
			op := &records.Operation{RecordID: rec.ID, ResourceID: o.ResourceID, Action: o.Action, AssignedComponent: o.AssignedComponent, InputHash: records.InputHash(o.Body)}
			if err := store.CreateOperation(ctx, op); err != nil {
				writeDataError(w, err)
				return
			}
			resp.Operations = append(resp.Operations, op)
		}
		writeJSON(w, http.StatusOK, resp)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, errorBody("MethodNotAllowed", fmt.Sprintf("%s is not supported", r.Method)))
	}
}

func writeDataError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, &database.ErrNotFound{}):
		writeJSON(w, http.StatusNotFound, errorBody("NotFound", err.Error()))
	case errors.Is(err, &database.ErrConcurrency{}):
		writeJSON(w, http.StatusPreconditionFailed, errorBody("PreconditionFailed", err.Error()))
	case errors.Is(err, &database.ErrInvalid{}):
		writeJSON(w, http.StatusBadRequest, errorBody("BadRequest", err.Error()))
	default:
		writeJSON(w, http.StatusInternalServerError, errorBody("Internal", err.Error()))
	}
}

func errorBody(code, message string) v1.ErrorResponse {
	return v1.ErrorResponse{Error: &v1.ErrorDetails{Code: code, Message: message}}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
