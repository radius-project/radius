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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
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

type internalIdentityClient struct {
	http.Handler
	client *http.Client
	origin string
}

// WithInternalIdentityClient connects receipt generation to the same client and
// transport used by the component's business calls to UCP.
func WithInternalIdentityClient(next http.Handler, options authz.Options, client *http.Client, endpoint string) (http.Handler, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || client == nil || parsed.Scheme != "http" || parsed.User != nil ||
		parsed.Host != "ucp."+options.Namespace+".svc:9444" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("internal identity evidence requires the configured canonical UCP client")
	}
	if next == nil {
		next = http.DefaultServeMux
	}
	return &internalIdentityClient{Handler: next, client: client, origin: "http://" + parsed.Host}, nil
}

func (c *internalIdentityClient) observe(ctx context.Context, options authz.Options, challenge string, now func() time.Time) (InternalIdentityReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	start := now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.origin+InternalIdentityPath+"?challenge="+challenge, nil)
	if err != nil {
		return InternalIdentityReceipt{}, errors.New("cannot construct internal identity observation")
	}
	request.Header.Set("Cache-Control", "no-store")
	client := *c.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return InternalIdentityReceipt{}, errors.New("UCP identity observation unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return InternalIdentityReceipt{}, errors.New("UCP identity observation failed")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	if err != nil || len(data) > 8192 {
		return InternalIdentityReceipt{}, errors.New("UCP identity receipt is unreadable or oversized")
	}
	receipt, err := DecodeInternalIdentityReceipt(data)
	if err != nil {
		return InternalIdentityReceipt{}, err
	}
	if receipt.SchemaVersion != 1 || receipt.Provider != "external" || receipt.Mode != options.EffectiveMode() ||
		receipt.Namespace != options.Namespace || receipt.Component != "ucp" ||
		receipt.Caller != "system:serviceaccount:"+options.Namespace+":"+options.Component ||
		receipt.Challenge != challenge || !validProofUUID(receipt.PodUID) || !validProofUUID(receipt.BootID) ||
		receipt.ObservedAt.Before(start.Add(-30*time.Second)) || receipt.ObservedAt.After(now().Add(30*time.Second)) {
		return InternalIdentityReceipt{}, errors.New("UCP identity receipt does not match this observation")
	}
	return receipt, nil
}

// DecodeInternalIdentityReceipt accepts only the exact, bounded base receipt
// schema. Callers must also verify its challenge, identity and observation time.
func DecodeInternalIdentityReceipt(data []byte) (InternalIdentityReceipt, error) {
	fail := errors.New("invalid UCP identity receipt")
	if len(data) > 8192 {
		return InternalIdentityReceipt{}, fail
	}
	fields := map[string]bool{"schemaVersion": false, "provider": false, "mode": false, "component": false,
		"namespace": false, "podUID": false, "bootID": false, "caller": false, "challenge": false, "observedAt": false}
	decoder := json.NewDecoder(bytes.NewReader(data))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return InternalIdentityReceipt{}, fail
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return InternalIdentityReceipt{}, fail
		}
		name, ok := token.(string)
		seen, known := fields[name]
		if !ok || !known || seen {
			return InternalIdentityReceipt{}, fail
		}
		fields[name] = true
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return InternalIdentityReceipt{}, fail
		}
		if name == "observedAt" {
			var timestamp string
			if json.Unmarshal(raw, &timestamp) != nil || !strings.HasSuffix(timestamp, "Z") {
				return InternalIdentityReceipt{}, fail
			}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return InternalIdentityReceipt{}, fail
	}
	if _, err := decoder.Token(); err != io.EOF {
		return InternalIdentityReceipt{}, fail
	}
	for _, present := range fields {
		if !present {
			return InternalIdentityReceipt{}, fail
		}
	}
	var receipt InternalIdentityReceipt
	if json.Unmarshal(data, &receipt) != nil {
		return InternalIdentityReceipt{}, fail
	}
	return receipt, nil
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
		evidence := struct {
			InternalIdentityReceipt
			Upstream *InternalIdentityReceipt `json:"upstream,omitempty"`
		}{InternalIdentityReceipt: receipt}
		if options.Component != "ucp" {
			client, ok := next.(*internalIdentityClient)
			if !ok {
				http.Error(w, "configured UCP client evidence unavailable", http.StatusServiceUnavailable)
				return
			}
			upstream, err := client.observe(r.Context(), options, receipt.Challenge, now)
			if err != nil {
				http.Error(w, "UCP identity evidence unavailable", http.StatusServiceUnavailable)
				return
			}
			evidence.Upstream = &upstream
			evidence.ObservedAt = now().UTC()
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(evidence); err != nil {
			ucplog.FromContextOrDiscard(r.Context()).Error(err, "cannot encode internal identity receipt")
		}
	})
}
