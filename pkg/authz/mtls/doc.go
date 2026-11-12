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

// Package mtls secures calls between Radius components with TLS server certificates and
// Kubernetes ServiceAccount tokens. No component presents a client certificate.
//
// A receiver (ucp, applications-rp, dynamic-rp) serves TLS with the csi-driver-spiffe
// certificate mounted in its pod, reloading it when it rotates. It does not request a client
// certificate; instead it verifies the caller's projected ServiceAccount token with
// pkg/authz/token and maps it to the caller's canonical "<component>.<namespace>.svc" identity.
//
// A caller verifies the receiver's certificate against the Radius CA bundle and requires
// exactly one SAN, a URI SAN with the receiver's SPIFFE ID
// spiffe://<trustDomain>/ns/<namespace>/sa/<receiver>.
// Only after that succeeds does it send the projected token whose audience is the receiver.
//
// The package name predates the switch from client certificates to tokens. Certificate
// failures are *Error values carrying an authz error code, such as PeerCertificateInvalid,
// for logging.
package mtls
