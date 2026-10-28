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
	"fmt"

	"go.yaml.in/yaml/v3"
)

// PodSecurityLevel is a Kubernetes Pod Security Standards level applied to environment namespaces.
type PodSecurityLevel string

const (
	// PodSecurityLevelBaseline is the default level. It blocks privileged containers, host namespaces,
	// hostPath volumes, and added capabilities outside the Baseline allowlist.
	PodSecurityLevelBaseline PodSecurityLevel = "baseline"

	// PodSecurityLevelRestricted additionally requires pods to drop all capabilities and run as non-root.
	PodSecurityLevelRestricted PodSecurityLevel = "restricted"
)

// Pod Security Admission namespace labels. Radius does not set the matching "-version" labels, so the
// cluster applies the latest policy version, which is the Kubernetes default.
const (
	PodSecurityLabelEnforce = "pod-security.kubernetes.io/enforce"
	PodSecurityLabelWarn    = "pod-security.kubernetes.io/warn"
	PodSecurityLabelAudit   = "pod-security.kubernetes.io/audit"
)

// defaultControlPlaneNamespace is used when Options.ControlPlaneNamespace is unset.
const defaultControlPlaneNamespace = "radius-system"

// protectedNamespaces are never labeled, even when an environment names them.
var protectedNamespaces = map[string]bool{
	"kube-system":     true,
	"kube-public":     true,
	"kube-node-lease": true,
}

// ParsePodSecurityLevel parses a Pod Security level. An empty string is PodSecurityLevelBaseline.
func ParsePodSecurityLevel(s string) (PodSecurityLevel, error) {
	switch PodSecurityLevel(s) {
	case "", PodSecurityLevelBaseline:
		return PodSecurityLevelBaseline, nil
	case PodSecurityLevelRestricted:
		return PodSecurityLevelRestricted, nil
	default:
		return "", fmt.Errorf("invalid pod security level %q: must be %q or %q", s, PodSecurityLevelBaseline, PodSecurityLevelRestricted)
	}
}

// UnmarshalYAML parses and validates a PodSecurityLevel from configuration.
func (l *PodSecurityLevel) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}

	parsed, err := ParsePodSecurityLevel(s)
	if err != nil {
		return err
	}

	*l = parsed
	return nil
}

// EffectivePodSecurityLevel returns the configured level, or PodSecurityLevelBaseline when it is unset.
func (o Options) EffectivePodSecurityLevel() PodSecurityLevel {
	if o.PodSecurityLevel == "" {
		return PodSecurityLevelBaseline
	}

	return o.PodSecurityLevel
}

// PodSecurityLabels returns the Pod Security Admission labels Radius applies to an environment or
// application namespace. It returns nil in off mode and for the control-plane and Kubernetes system
// namespaces. dryRun sets only the warn and audit labels; enforce also sets the enforce label.
func (o Options) PodSecurityLabels(namespace string) map[string]string {
	mode := o.EffectiveMode()
	if mode == ModeOff {
		return nil
	}

	if namespace == o.EffectiveControlPlaneNamespace() || protectedNamespaces[namespace] {
		return nil
	}

	level := string(o.EffectivePodSecurityLevel())
	labels := map[string]string{
		PodSecurityLabelWarn:  level,
		PodSecurityLabelAudit: level,
	}
	if mode == ModeEnforce {
		labels[PodSecurityLabelEnforce] = level
	}

	return labels
}

// MissingLabels returns the entries of want whose keys are not set in existing, or nil when every
// key is already set. Existing values are never compared, so operator-set labels are kept.
func MissingLabels(existing, want map[string]string) map[string]string {
	var missing map[string]string
	for k, v := range want {
		if _, ok := existing[k]; ok {
			continue
		}
		if missing == nil {
			missing = map[string]string{}
		}
		missing[k] = v
	}

	return missing
}
