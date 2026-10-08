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

// Package authz contains the shared authorization mode setting used by Radius
// services for internal component authorization.
//
// Every service reads a Mode from its configuration file:
//
//   - ModeOff (default): authorization checks are not applied.
//   - ModeDryRun: checks run, and denials are logged instead of rejected. This is
//     the preflight run operators use before turning on enforcement.
//   - ModeEnforce: denials are rejected with a specific error code.
//
// Checks build a Decision and pass it to Apply, which acts on it according to the
// configured mode. An enforced denial is a *DeniedError, which converts to an ARM error
// response whose HTTP status comes from StatusForCode. Conversion requires the
// caller's requested action and target, names the admission rule for
// AdmissionPolicyDenied, and omits the internal diagnostic reason.
package authz
