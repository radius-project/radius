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

package sdk

import (
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// NewClientOptions creates a new ARM client options object with the given connection's endpoint, audience, transport and
// removes the authorization header policy.
func NewClientOptions(connection Connection) *arm.ClientOptions {
	return &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.Configuration{
				Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
					cloud.ResourceManager: {
						Endpoint: connection.Endpoint(),
						Audience: "https://management.core.windows.net",
					},
				},
			},
			PerRetryPolicies: []policy.Policy{
				// Autorest will inject an empty bearer token, which conflicts with bearer auth
				// when its used by Kubernetes. We don't *ever* need Autorest to handle auth for us
				// so we just remove it.
				//
				// We'll solve this problem permanently by writing our own client.
				&removeAuthorizationHeaderPolicy{},
			},
			Transport: connection.Client(),
			// When updating azcore to 1.11.1 from 1.7.0, we saw that HTTPS check for Authentication was added.
			// Link to the check: https://github.com/Azure/azure-sdk-for-go/blob/main/sdk/azcore/runtime/policy_bearer_token.go#L118
			//
			// This check was failing for some unit tests because the ARM requests are made over HTTP and the bearer token is being sent in the header.
			// This is a temporary fix to allow sending the bearer token over HTTP.
			// We don't have any use cases where we send the bearer token over HTTP in production.
			InsecureAllowCredentialWithHTTP: true,
		},
		DisableRPRegistration: true,
	}
}

var _ policy.Policy = (*removeAuthorizationHeaderPolicy)(nil)

type removeAuthorizationHeaderPolicy struct {
}

// Do removes the Authorization header from the request before sending it to the next policy.
func (p *removeAuthorizationHeaderPolicy) Do(req *policy.Request) (*http.Response, error) {
	delete(req.Raw().Header, "Authorization")
	return req.Next()
}
