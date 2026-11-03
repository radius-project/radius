// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

func inspectionRoute(path string) (operation, scope string) {
	lower := strings.ToLower(path)
	const prefix = "/providers/radius.core/"
	index := strings.LastIndex(lower, prefix)
	if index < 0 {
		return "", ""
	}
	operation = lower[index+len(prefix):]
	scope = path[:index]
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
			path := r.URL.Path
			if pathBase != "" {
				if !strings.HasPrefix(path, pathBase+"/") {
					next.ServeHTTP(w, r)
					return
				}
				path = strings.TrimPrefix(path, pathBase)
			}
			operation, scope := inspectionRoute(path)
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
				var request api.AccessCheckRequest
				request, err = decodeAccessCheck(w, r)
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

// Generated model unmarshallers intentionally ignore unknown properties. Decode
// a strict wire shape instead so groups cannot be silently accepted as input.
func decodeAccessCheck(w http.ResponseWriter, r *http.Request) (api.AccessCheckRequest, error) {
	var wire struct {
		Principal *struct {
			Type        *api.PrincipalType `json:"type"`
			Issuer      *string            `json:"issuer"`
			Subject     *string            `json:"subject"`
			DisplayName *string            `json:"displayName,omitempty"`
		} `json:"principal,omitempty"`
		Checks []*struct {
			Permission *string `json:"permission"`
			Target     *string `json:"target"`
		} `json:"checks"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return api.AccessCheckRequest{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return api.AccessCheckRequest{}, errors.New("expected exactly one JSON request object")
	}
	request := api.AccessCheckRequest{}
	if wire.Principal != nil {
		request.Principal = &api.Principal{
			Type: wire.Principal.Type, Issuer: wire.Principal.Issuer,
			Subject: wire.Principal.Subject, DisplayName: wire.Principal.DisplayName,
		}
	}
	for _, check := range wire.Checks {
		if check == nil {
			return request, errors.New("access check cannot be null")
		}
		request.Checks = append(request.Checks, &api.AccessCheck{Permission: check.Permission, Target: check.Target})
	}
	return request, nil
}

func writeInspectionError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v1.ErrorResponse{Error: &v1.ErrorDetails{Code: code, Message: message}}); err != nil {
		ucplog.FromContextOrDiscard(r.Context()).Error(err, "Failed to write inspection error")
	}
}
