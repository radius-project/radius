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

package tokencredentials

import (
	"context"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

var _ azcore.TokenCredential = &AnonymousCredential{}

type AnonymousCredential struct {
}

// PolicyFunc is a type that implements the Policy interface.
// Use this type when implementing a stateless policy as a first-class function.
type PolicyFunc func(*policy.Request) (*http.Response, error)

// Do implements the Policy interface on PolicyFunc.
func (pf PolicyFunc) Do(req *policy.Request) (*http.Response, error) {
	return pf(req)
}

func (*AnonymousCredential) NewAuthenticationPolicy(options policy.BearerTokenOptions) policy.Policy {
	return PolicyFunc(func(req *policy.Request) (*http.Response, error) {
		return req.Next()
	})
}

func (a *AnonymousCredential) GetToken(ctx context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{}, nil
}
