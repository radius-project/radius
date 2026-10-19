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

// Package mtls loads a Radius component's mTLS certificate, key, and CA bundle from
// mounted files, reloads them when they change, and builds server and client TLS
// configs. A verified peer is mapped to a canonical component identity,
// "<component>.<namespace>.svc", from its DNS SANs; the subject common name is
// ignored. Certificate failures are *Error values carrying an authz error code,
// such as PeerCertificateInvalid, for logging.
package mtls
