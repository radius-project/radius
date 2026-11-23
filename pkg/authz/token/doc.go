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

// Package token verifies the Kubernetes ServiceAccount tokens that Radius components
// send to each other as "Authorization: Bearer <token>".
//
// A Verifier checks tokens offline against the cluster's ServiceAccount signing keys,
// which it reads from the API server's OIDC discovery endpoint and caches. It refetches
// the keys periodically and when a token names an unknown key ID, at a limited rate.
// If the keys cannot be refreshed for too long, every token is rejected with
// AuthorizationUnavailable until a refresh succeeds. A verified token maps to the
// calling component's canonical "<component>.<namespace>.svc" identity.
package token
