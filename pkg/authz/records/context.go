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

package records

import (
	"context"
	"net/http"
)

// IDs are the record and operation a component is working on.
type IDs struct {
	RecordID    string
	OperationID string
}

type idsKey struct{}
type sourceNamespaceKey struct{}
type userApprovalKey struct{}

// ResourceApproval binds one literal template resource to an authorized input.
type ResourceApproval struct {
	ResourceID string `json:"resourceId"`
	Action     string `json:"action"`
	InputHash  string `json:"inputHash"`
}

// UserApproval is issued in UCP after user checks, never deserialized from headers.
type UserApproval struct {
	// Identity is JSON text so database map decoding cannot reinterpret it.
	Identity  string             `json:"identityJSON"`
	Resources []ResourceApproval `json:"resources"`
}

// WithUserApproval carries preflight approval to UCP's record creation path.
func WithUserApproval(ctx context.Context, approval UserApproval) context.Context {
	return context.WithValue(ctx, userApprovalKey{}, approval)
}

// UserApprovalFromContext returns only a server-issued approval.
func UserApprovalFromContext(ctx context.Context) *UserApproval {
	approval, ok := ctx.Value(userApprovalKey{}).(UserApproval)
	if !ok {
		return nil
	}
	return &approval
}

// WithIDs returns a context that carries ids.
func WithIDs(ctx context.Context, ids IDs) context.Context {
	return context.WithValue(ctx, idsKey{}, ids)
}

// IDsFromContext returns the IDs set by WithIDs.
func IDsFromContext(ctx context.Context) IDs {
	ids, _ := ctx.Value(idsKey{}).(IDs)
	return ids
}

// WithSourceNamespace returns a context that carries the Kubernetes namespace the
// controller acts for.
func WithSourceNamespace(ctx context.Context, namespace string) context.Context {
	return context.WithValue(ctx, sourceNamespaceKey{}, namespace)
}

// SourceNamespaceFromContext returns the namespace set by WithSourceNamespace.
func SourceNamespaceFromContext(ctx context.Context) string {
	ns, _ := ctx.Value(sourceNamespaceKey{}).(string)
	return ns
}

// HeaderTransport adds the record, operation, and source namespace from the request
// context to outgoing requests (B4, C2).
type HeaderTransport struct {
	Inner http.RoundTripper
}

// RoundTrip implements http.RoundTripper.
func (t *HeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ids := IDsFromContext(req.Context())
	ns := SourceNamespaceFromContext(req.Context())
	if ids.RecordID != "" || ids.OperationID != "" || ns != "" {
		req = req.Clone(req.Context())
		if ids.RecordID != "" {
			req.Header.Set(HeaderRecord, ids.RecordID)
		}
		if ids.OperationID != "" {
			req.Header.Set(HeaderOperation, ids.OperationID)
		}
		if ns != "" {
			req.Header.Set(HeaderSourceNamespace, ns)
		}
	}
	inner := t.Inner
	if inner == nil {
		inner = http.DefaultTransport
	}
	return inner.RoundTrip(req)
}
