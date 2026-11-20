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

package clientv2

import (
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// WithForceDelete returns a copy of opts that adds force=true to every DELETE request. Radius accepts force=true
// on a resource DELETE to skip the provisioning state check and recipe cleanup it can't confirm.
func WithForceDelete(opts arm.ClientOptions) arm.ClientOptions {
	opts.PerCallPolicies = append(
		append([]policy.Policy{}, opts.PerCallPolicies...),
		&forceDeletePolicy{},
	)
	return opts
}

// forceDeletePolicy is a per-call pipeline policy that appends force=true to DELETE request URL query strings.
type forceDeletePolicy struct{}

func (p *forceDeletePolicy) Do(req *policy.Request) (*http.Response, error) {
	rawReq := req.Raw()
	if rawReq.Method != http.MethodDelete {
		return req.Next()
	}

	q := rawReq.URL.Query()
	if _, ok := q["force"]; ok {
		return req.Next()
	}

	q.Set("force", "true")
	rawReq.URL.RawQuery = q.Encode()
	return req.Next()
}
