// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdb

import (
	"net/http"
	"net/url"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

var (
	// AuthorizationHeaderKey is the header key of Authorization Header.
	AuthorizationHeaderKey = http.CanonicalHeaderKey("Authorization")
)

// ClientTransport is the custom transport to support azure ad authentication and logging.
type ClientTransport struct {
	tokenCreds          azcore.TokenCredential
	tokenRequestOptions policy.TokenRequestOptions
}

// NewClientTransport creates new ClientTransport object.
func NewClientTransport(authOptions *AzureADAuthOptions) (*ClientTransport, error) {
	transport := &ClientTransport{}

	clientOps := azcore.ClientOptions{
		Cloud: cloud.Configuration{
			LoginEndpoint: authOptions.Endpoint,
			Services:      map[cloud.ServiceName]cloud.ServiceConfiguration{},
		},
	}

	if authOptions != nil && authOptions.ClientID != "" {
		if authOptions.ClientSecret != "" {
			// Use ClientSecret authentication for dev/test purpose.
			ops := &azidentity.ClientSecretCredentialOptions{ClientOptions: clientOps}
			var err error
			transport.tokenCreds, err = azidentity.NewClientSecretCredential(
				authOptions.TenantID,
				authOptions.ClientID,
				authOptions.ClientSecret,
				ops)
			if err != nil {
				return nil, err
			}
		} else {
			// Use managed identity authentication.
			ops := &azidentity.ManagedIdentityCredentialOptions{ClientOptions: clientOps}
			ops.ID = azidentity.ClientID(authOptions.ClientID)
			var err error
			transport.tokenCreds, err = azidentity.NewManagedIdentityCredential(ops)
			if err != nil {
				return nil, err
			}
		}

		transport.tokenRequestOptions = policy.TokenRequestOptions{
			Scopes:   []string{authOptions.Audience},
			TenantID: authOptions.TenantID,
		}
	}

	return transport, nil
}

// RoundTrip fetches Bearer token from AAD and injects bearer token to Authorization header.
// TODO: emit dependency metrics and traces here.
func (t *ClientTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.tokenCreds != nil {
		token, err := t.tokenCreds.GetToken(req.Context(), t.tokenRequestOptions)
		if err != nil {
			return nil, err
		}
		auth := url.QueryEscape("type=aad&ver=1.0&sig=" + token.Token)
		req.Header.Set(AuthorizationHeaderKey, auth)
	}

	return t.base().RoundTrip(req)
}

func (t *ClientTransport) base() http.RoundTripper {
	return http.DefaultTransport
}

// CancelRequest cancels an in-flight request by closing its connection.
func (t *ClientTransport) CancelRequest(req *http.Request) {
	type canceler interface {
		CancelRequest(*http.Request)
	}
	if cr, ok := t.base().(canceler); ok {
		cr.CancelRequest(req)
	}
}
