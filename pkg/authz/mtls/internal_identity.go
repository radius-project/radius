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

package mtls

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// InternalIdentityPath returns a fresh, authenticated receiver assertion. Its
// authenticity depends on operator-secured transport, not on server TLS supplied by Radius.
const InternalIdentityPath = "/healthz/internal-identity"

// InternalIdentityReceipt binds a verified caller and challenge to one receiver
// process. It contains no token material and does not attest outbound client use.
type InternalIdentityReceipt struct {
	SchemaVersion int        `json:"schemaVersion"`
	Provider      string     `json:"provider"`
	Mode          authz.Mode `json:"mode"`
	Component     string     `json:"component"`
	Namespace     string     `json:"namespace"`
	PodUID        string     `json:"podUID"`
	BootID        string     `json:"bootID"`
	Caller        string     `json:"caller"`
	Challenge     string     `json:"challenge"`
	ObservedAt    time.Time  `json:"observedAt"`
}

func internalIdentityHandler(next http.Handler, options authz.Options, podUID string, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != InternalIdentityPath {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		identity, verified := IdentityFromContext(r.Context())
		if !verified || identity.Namespace != options.Namespace || !validComponent(identity.Component) {
			http.Error(w, "verified internal identity required", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(query) != 1 || len(query["challenge"]) != 1 || !validProofUUID(query.Get("challenge")) {
			http.Error(w, "one canonical UUID challenge is required", http.StatusBadRequest)
			return
		}
		if !validProofUUID(podUID) || !validProofUUID(processBootID) ||
			options.EffectiveMTLSProvider() != "external" || options.EffectiveMode() == authz.ModeOff {
			http.Error(w, "internal identity evidence unavailable", http.StatusServiceUnavailable)
			return
		}
		receipt := InternalIdentityReceipt{
			SchemaVersion: 1, Provider: "external", Mode: options.EffectiveMode(), Component: options.Component,
			Namespace: options.Namespace, PodUID: podUID, BootID: processBootID,
			Caller:    "system:serviceaccount:" + identity.Namespace + ":" + identity.Component,
			Challenge: query.Get("challenge"), ObservedAt: now().UTC(),
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(receipt); err != nil {
			ucplog.FromContextOrDiscard(r.Context()).Error(err, "cannot encode internal identity receipt")
		}
	})
}
