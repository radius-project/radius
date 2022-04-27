// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicecontext

type contextKey struct {
	name string
}

func (k *contextKey) String() string {
	return "radius context name: " + k.name
}

var (
	// armContextKey is the context key for ARM RPC request.
	armContextKey = &contextKey{"armrpc"}

	// HostingConfigContextKey is the context key for hosting configuration.
	HostingConfigContextKey = &contextKey{"hostingConfig"}
)
