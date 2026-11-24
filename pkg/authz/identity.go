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

package authz

import "context"

// Components lists the Radius components that may call another component. The names
// match the Kubernetes ServiceAccount and Service names in the Helm chart.
var Components = []string{"ucp", "applications-rp", "dynamic-rp", "controller", "bicep-de"}

// Identity is a verified Radius component identity.
type Identity struct {
	// Component is the component name, such as "applications-rp".
	Component string

	// Namespace is the Radius namespace.
	Namespace string
}

// String returns the canonical identity, "<component>.<namespace>.svc".
func (i Identity) String() string {
	return i.Component + "." + i.Namespace + ".svc"
}

type identityKey struct{}

// WithIdentity returns a copy of ctx that carries the caller's identity.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// IdentityFromContext returns the caller's identity stored by WithIdentity.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}
