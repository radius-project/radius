// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdb

import (
	"net/http"
	"net/url"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

var AuthorizationHeader = http.CanonicalHeaderKey("Authorization")

// AzureIdentityTransport is an http.RoundTripper that injects bearer token to outgoing request.
type AzureIdentityTransport struct {
	tokenCreds azcore.TokenCredential
}

func NewAzureIdentityTransport(clientID string) (*AzureIdentityTransport, error) {
	transport := &AzureIdentityTransport{}

	// TODO: support service principal.
	if clientID != "" {
		ops := &azidentity.ManagedIdentityCredentialOptions{}
		ops.ID = azidentity.ClientID
		var err error
		if transport.tokenCreds, err = azidentity.NewManagedIdentityCredential(clientID, ops); err != nil {
			return nil, err
		}
	}

	return transport, nil
}

// RoundTrip implements http.RoundTripper
func (t *AzureIdentityTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.tokenCreds != nil {
		token, err := t.tokenCreds.GetToken(req.Context(), policy.TokenRequestOptions{Scopes: []string{"https://management.core.windows.net//.default"}})
		if err != nil {
			return nil, err
		}
		auth := url.QueryEscape("type=aad&ver=1.0&sig=" + token.Token)
		req.Header.Set(AuthorizationHeader, auth)
	}

	return t.base().RoundTrip(req)
}

func (t *AzureIdentityTransport) base() http.RoundTripper {
	return http.DefaultTransport
}

// CancelRequest cancels an in-flight request by closing its connection.
func (t *AzureIdentityTransport) CancelRequest(req *http.Request) {
	type canceler interface {
		CancelRequest(*http.Request)
	}
	if cr, ok := t.base().(canceler); ok {
		cr.CancelRequest(req)
	}
}
