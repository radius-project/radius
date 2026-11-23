// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

func inspectionRoute(path, pathBase string) (operation, scope string) {
	path, mounted := MountedRequestPath(path, pathBase)
	if !mounted {
		return "", ""
	}
	lower := strings.ToLower(strings.TrimSuffix(path, "/"))
	const prefix = "/providers/radius.core/"
	index := strings.LastIndex(lower, prefix)
	if index < 0 {
		return "", ""
	}
	operation = lower[index+len(prefix):]
	scope = lower[:index]
	if scope == "" {
		scope = "/"
	}
	if operation == "checkaccess" || operation == "whoami" && scope == "/" {
		return operation, scope
	}
	return "", ""
}

// Inspection serves only the two trusted inspection operations before generic
// resource/proxy dispatch. Authentication must run first, in every workload mode.
func Inspection(runtime *PolicyRuntime, pathBase string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			operation, scope := inspectionRoute(r.URL.Path, pathBase)
			if operation == "" {
				next.ServeHTTP(w, r)
				return
			}
			identity, verified := IdentityFromContext(r.Context())
			if !verified {
				writeInspectionError(w, r, http.StatusUnauthorized, "InvalidAuthenticationInfo", "A verified Kubernetes caller identity is required.")
				return
			}
			method := http.MethodPost
			if operation == "whoami" {
				method = http.MethodGet
			}
			if r.Method != method {
				w.Header().Set("Allow", method)
				writeInspectionError(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "This inspection operation requires "+method+".")
				return
			}
			if r.URL.Query().Get("api-version") != "2025-08-01-preview" {
				writeInspectionError(w, r, http.StatusBadRequest, "InvalidApiVersionParameter", "Use api-version=2025-08-01-preview.")
				return
			}
			var result any
			var err error
			if operation == "whoami" {
				result, err = runtime.WhoAmI(identity)
			} else {
				var request api.BoundedAccessCheckRequest
				request, err = decodeAccessCheck(w, r)
				if err == nil {
					_, err = validateAccessCheck(scope, request)
				}
				if err == nil {
					result, err = runtime.CheckAccess(identity, scope, request)
				}
			}
			switch {
			case errors.Is(err, ErrAuthorizationUnavailable):
				writeAuthorizationUnavailable(w, r)
			case errors.Is(err, ErrCheckAccessDenied):
				writeInspectionError(w, r, http.StatusForbidden, "AuthorizationFailed", err.Error())
			case errors.Is(err, ErrMembershipUnavailable):
				writeInspectionError(w, r, http.StatusServiceUnavailable, "AuthorizationIndeterminate", err.Error())
			case err != nil:
				writeInspectionError(w, r, http.StatusBadRequest, "InvalidRequestContent", err.Error())
			default:
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(result); err != nil {
					ucplog.FromContextOrDiscard(r.Context()).Error(err, "Failed to write inspection response")
				}
			}
		})
	}
}

// Validate exact wire keys before generated decoding, which ignores unknown
// properties. Struct decoding would also accept case variants of trusted fields.
func decodeAccessCheck(w http.ResponseWriter, r *http.Request) (api.BoundedAccessCheckRequest, error) {
	var body json.RawMessage
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&body); err != nil {
		return api.BoundedAccessCheckRequest{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return api.BoundedAccessCheckRequest{}, errors.New("expected exactly one JSON request object")
	}
	request := api.BoundedAccessCheckRequest{}
	wire, err := inspectionRequestObject(body, "principal", "checks")
	if err != nil {
		return request, err
	}
	var checks []json.RawMessage
	if raw := wire["checks"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &checks); err != nil {
			return request, err
		}
	}
	if len(checks) == 0 || len(checks) > maxAccessChecks {
		return request, fmt.Errorf("checks must contain between 1 and %d items", maxAccessChecks)
	}
	if raw := wire["principal"]; len(raw) > 0 {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return request, errors.New("principal must not be null; omit it to check the caller")
		}
		principal, err := inspectionRequestObject(raw, "type", "issuer", "subject", "displayName")
		if err != nil {
			return request, err
		}
		if bytes.Equal(bytes.TrimSpace(principal["displayName"]), []byte("null")) {
			return request, errors.New("principal.displayName must not be null; omit it if no label is supplied")
		}
		if err := json.Unmarshal(raw, &request.Principal); err != nil {
			return request, err
		}
	}
	for _, raw := range checks {
		if _, err := inspectionRequestObject(raw, "permission", "target"); err != nil {
			return request, err
		}
		var check api.BoundedAccessCheck
		if err := json.Unmarshal(raw, &check); err != nil {
			return request, err
		}
		request.Checks = append(request.Checks, &check)
	}
	return request, nil
}

func inspectionRequestObject(raw json.RawMessage, allowed ...string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if token != json.Delim('{') {
		return nil, errors.New("expected a JSON object")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, errors.New("expected a JSON member name")
		}
		if !slices.Contains(allowed, name) {
			return nil, fmt.Errorf("unknown field %q", name)
		}
		if _, exists := fields[name]; exists {
			return nil, fmt.Errorf("duplicate field %q", name)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return fields, nil
}

func writeInspectionError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v1.ErrorResponse{Error: &v1.ErrorDetails{Code: code, Message: message}}); err != nil {
		ucplog.FromContextOrDiscard(r.Context()).Error(err, "Failed to write inspection error")
	}
}
