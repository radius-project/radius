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

import (
	"context"
	"slices"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

// Admission rule identifiers. They match the rule names in the chart's
// radius-control-plane-isolation admission policy.
const (
	// RuleControlPlaneNamespace rejects environments, applications and workloads in the
	// Radius control-plane namespace.
	RuleControlPlaneNamespace = "control-plane-namespace"

	// RuleControlPlaneSecret rejects workloads that reference a Radius control-plane Secret.
	RuleControlPlaneSecret = "control-plane-secret"
)

// EffectiveControlPlaneNamespace returns the configured control-plane namespace, or
// "radius-system" when it is unset.
func (o Options) EffectiveControlPlaneNamespace() string {
	if o.ControlPlaneNamespace == "" {
		return defaultControlPlaneNamespace
	}

	return o.ControlPlaneNamespace
}

// IsControlPlaneNamespace reports whether namespace is the Radius control-plane namespace.
func (o Options) IsControlPlaneNamespace(namespace string) bool {
	return strings.EqualFold(namespace, o.EffectiveControlPlaneNamespace())
}

// IsControlPlaneSecret reports whether name is one of the configured control-plane Secrets.
func (o Options) IsControlPlaneSecret(name string) bool {
	return name != "" && slices.Contains(o.ControlPlaneSecrets, name)
}

// AdmitNamespace applies the control-plane-namespace rule to the Kubernetes namespace that an
// environment or application requested. action and target identify the request. It returns nil
// in off mode, logs a would-deny in dryRun, and returns an AdmissionPolicyDenied *DeniedError in
// enforce.
func (o Options) AdmitNamespace(ctx context.Context, action, target, namespace string) error {
	mode := o.EffectiveMode()
	if mode == ModeOff {
		return nil
	}

	return Apply(ctx, mode, Decision{
		Allowed: !o.IsControlPlaneNamespace(namespace),
		Code:    v1.CodeAdmissionPolicyDenied,
		Action:  action,
		Target:  target,
		Rule:    RuleControlPlaneNamespace,
	})
}
