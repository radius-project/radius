// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	armAuthenticator "github.com/project-radius/radius/pkg/corerp/authentication"
	"github.com/project-radius/radius/pkg/radlogger"
)

const (
	IngressCertThumbprintHeader = "X-SSL-Client-Thumbprint"
	ArmCertificateRefreshRate   = 1 * time.Hour
	ContentTypeHeaderKey        = "Content-Type"
	ApplicationJson             = "application/json"
)

var WhiteListEps = []string{"http://:8080/healthz", "http://:8080/version"}

// armErrorResponse is for setting the response struct as per ARM specs
// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/common-api-details.md
type armErrorResponse struct {
	Error  *err `json:"error"`
	Status int  `json:"-"`
}
type err struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ClientCertValidator validates the thumbprint received in the request header with
// the active thumbprints fetched from ARM Metadata endpoint
func ClientCertValidator(armCertMgr *armAuthenticator.ArmCertManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//skip cert validation for health and version endpoint
			log := radlogger.GetLogger(r.Context())
			for _, whiteListEp := range WhiteListEps {
				if strings.Compare(r.URL.Path, whiteListEp) == 0 {
					next.ServeHTTP(w, r)
					return
				}
			}
			clientRequestThumbprint := r.Header.Get(IngressCertThumbprintHeader)
			if clientRequestThumbprint == "" {
				log.V(radlogger.Error).Info("X-SSL-Client-Thumbprint header is missing")
				writeUnauthorizedResp(w)
				return
			}
			isValid, err := armCertMgr.IsValidThumbprint(clientRequestThumbprint)
			if err != nil || !isValid {
				msg := fmt.Sprintf("Error validating the thumbprint %v", err)
				log.V(radlogger.Error).Info(msg)
				writeUnauthorizedResp(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeUnauthorizedResp(w http.ResponseWriter) {
	w.Header().Set(ContentTypeHeaderKey, ApplicationJson)
	er := &err{
		Code:    fmt.Sprintf("%d", http.StatusUnauthorized),
		Message: "Unauthorized",
	}
	errResp := &armErrorResponse{
		Status: http.StatusUnauthorized,
		Error:  er,
	}
	err := json.NewEncoder(w).Encode(errResp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(`{"error": {"code": "500","message": "operation failed"}}`)
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
}
