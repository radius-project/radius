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
// component authorization design (eng/design-notes/security/internal-component-authorization.md).
// Each is enabled by the pull request that implements the boundary. Expected results use the
// design's error codes; see pkg/authz for their HTTP status codes.

// Expected: a pod without a Radius component certificate calling applications-rp directly
// fails the mTLS handshake (logged as PeerCertificateInvalid) or is rejected with
// InvalidAuthenticationInfo (401).
func Test_Boundary_UnauthenticatedCallToApplicationsRP(t *testing.T) {
	t.Skip("enabled by A4")
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

// Expected: an application workload that references a control-plane Secret is rejected
// by admission with AdmissionPolicyDenied (403).
func Test_Boundary_ControlPlaneSecretReferenceDenied(t *testing.T) {
	t.Skip("enabled by E2")
}
