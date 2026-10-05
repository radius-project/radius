/*
Copyright 2026 The Radius Authors.

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

package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/logging"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// Log-only codes for mTLS handshake failures. A failed handshake has no HTTP response,
// so these codes are used only in logs and metrics and have no HTTP status.
const (
	// CodePeerCertificateInvalid is logged when a peer's certificate fails verification.
	CodePeerCertificateInvalid = "PeerCertificateInvalid"

	// CodeCertificateExpired is logged when a component's own certificate has expired.
	CodeCertificateExpired = "CertificateExpired"
)

var statusForCode = map[string]int{
	v1.CodeInvalidAuthenticationInfo: http.StatusUnauthorized,
	v1.CodeAuthorizationFailed:       http.StatusForbidden,
	v1.CodeExecutionRecordNotActive:  http.StatusForbidden,
	v1.CodeGrantScopeExceeded:        http.StatusForbidden,
	v1.CodeOperationNotAssigned:      http.StatusForbidden,
	v1.CodeOperationInputMismatch:    http.StatusConflict,
	v1.CodeQueueWaitLimitExceeded:    http.StatusForbidden,
	v1.CodeCredentialIssuanceDenied:  http.StatusForbidden,
	v1.CodeAdmissionPolicyDenied:     http.StatusForbidden,
	v1.CodeAuthorizationUnavailable:  http.StatusServiceUnavailable,
}

// StatusForCode returns the HTTP status for an authorization error code. It returns
// http.StatusInternalServerError for log-only codes and unknown codes, since neither
// is expected in an HTTP response.
func StatusForCode(code string) int {
	if status, ok := statusForCode[code]; ok {
		return status
	}
	return http.StatusInternalServerError
}

// code returns the denial's error code, defaulting to AuthorizationFailed when unset.
func (e *DeniedError) code() string {
	if e.Code == "" {
		return v1.CodeAuthorizationFailed
	}
	return e.Code
}

// ErrorResponse converts the denial to an ARM error response body. The message names
// the action and target, and includes Reason as provided by the check.
func (e *DeniedError) ErrorResponse() v1.ErrorResponse {
	message := "Authorization denied"
	if e.Action != "" || e.Target != "" {
		message = fmt.Sprintf("Authorization denied for action '%s' on target '%s'", e.Action, e.Target)
	}
	if e.Reason != "" {
		message += ": " + e.Reason
	}

	return v1.ErrorResponse{
		Error: &v1.ErrorDetails{
			Code:    e.code(),
			Message: message,
			Target:  e.Target,
		},
	}
}

// Response converts the denial to a rest.Response with the HTTP status for its code.
func (e *DeniedError) Response() rest.Response {
	return &DeniedResponse{
		StatusCode: StatusForCode(e.code()),
		Body:       e.ErrorResponse(),
	}
}

// DeniedResponse is a rest.Response that writes an ARM error payload for an authorization failure.
type DeniedResponse struct {
	// StatusCode is the HTTP status to write.
	StatusCode int

	// Body is the ARM error payload.
	Body v1.ErrorResponse
}

// Apply writes the response status and JSON body to w.
func (r *DeniedResponse) Apply(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("responding with status code: %d", r.StatusCode), logging.LogHTTPStatusCode, r.StatusCode)

	bytes, err := json.MarshalIndent(r.Body, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling %T: %w", r.Body, err)
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(r.StatusCode)
	if _, err := w.Write(bytes); err != nil {
		return fmt.Errorf("error writing marshaled %T bytes to output: %w", r.Body, err)
	}

	return nil
}
