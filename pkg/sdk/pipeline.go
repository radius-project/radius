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
