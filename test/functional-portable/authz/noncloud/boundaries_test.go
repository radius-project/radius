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

import "testing"

// The tests below are placeholders for the trust boundaries described in the internal
// component authorization design (eng/design-notes/security/2026-09-internal-component-authorization.md).
// Each is enabled by the pull request that implements the boundary. Expected results use the
// design's error codes; see pkg/authz for their HTTP status codes.

// Expected: a pod calling applications-rp directly without a ServiceAccount token is
// rejected with InvalidAuthenticationInfo (401), or logged as a would-deny in dryRun.
//
// The test calls applications-rp from a rogue pod rather than through UCP, which attaches
// its own token.
func Test_Boundary_UnauthenticatedCallToApplicationsRP(t *testing.T) {
	testUnauthenticatedCallToApplicationsRP(t)
}

// Expected: a valid token for a component that is not an allowed caller of applications-rp
// (dynamic-rp; only UCP is allowed) is rejected with AuthorizationFailed (403), or logged as
// a would-deny in dryRun.
func Test_Boundary_DisallowedCaller(t *testing.T) {
	testDisallowedCaller(t)
}

// Expected: a ServiceAccount token issued for one receiver and replayed to another component
// is rejected with InvalidAuthenticationInfo (401) because the audience does not match.
func Test_Boundary_TokenReplay(t *testing.T) {
	testTokenReplay(t)
}

// Expected: a Radius client calling a server whose certificate carries another component's
// SPIFFE ID fails the handshake with PeerCertificateInvalid and sends no token.
func Test_Boundary_WrongServerSPIFFEID(t *testing.T) {
	testWrongServerSPIFFEID(t)
}

// Expected: a request to UCP carrying a forged x-remote-user header, without a verified
// Kubernetes proxy certificate, is rejected with InvalidAuthenticationInfo (401).
func Test_Boundary_ForgedRemoteUserHeader(t *testing.T) {
	t.Skip("enabled by A6")
}

// Expected: a deployment engine request for a resource outside the execution record's
// actions or targets is rejected by UCP with GrantScopeExceeded (403).
func Test_Boundary_RecordScopeExceeded(t *testing.T) {
	t.Skip("enabled by B3")
}

// Expected: a resource provider or worker receiving inputs that do not hash to the
// operation's inputHash rejects the work with OperationInputMismatch (409).
func Test_Boundary_OperationInputMismatch(t *testing.T) {
	t.Skip("enabled by B5")
}

// Expected: a controller object in a namespace that is not mapped to the target resource
// group or environment is rejected with AuthorizationFailed (403) before any change starts.
func Test_Boundary_ControllerNamespaceMapping(t *testing.T) {
	t.Skip("enabled by C3")
}

// Expected: a provider writing to the data-access service for an operation it is not
// assigned to is rejected with OperationNotAssigned (403).
func Test_Boundary_UnassignedDataAccessWrite(t *testing.T) {
	t.Skip("enabled by D2")
}

// Expected: an application template that creates a ClusterRole is denied by the
// namespace-limited template identity with AuthorizationFailed (403).
func Test_Boundary_TemplateClusterRoleDenied(t *testing.T) {
	t.Skip("enabled by D7")
}
