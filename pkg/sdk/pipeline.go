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
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
)

const (
	// module is used to build a runtime.Pipeline. This is informational text about the client that
	// is added as part of the User-Agent header.
	module = "v20220315privatepreview"

	// version is used to build a runtime.Pipeline. This is informational text about the client that
	// is added as part of the User-Agent header.
	version = "v0.0.1"
)

// NewPipeline builds a runtime.Pipeline from a Radius SDK connection. This is used to construct
// autorest Track2 Go clients.
func NewPipeline(connection Connection) runtime.Pipeline {
	return runtime.NewPipeline(module, version, runtime.PipelineOptions{}, &NewClientOptions(connection).ClientOptions)
}

// NewClientOptions builds an arm.ClientOptions from a Radius SDK connection. This is used
// to construct autorest Track2 Go clients.
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

func (p *removeAuthorizationHeaderPolicy) Do(req *policy.Request) (*http.Response, error) {
	delete(req.Raw().Header, "Authorization")
	return req.Next()
}
