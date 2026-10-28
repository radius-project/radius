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

package deployment

import (
	"context"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/resourcemodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// admitOutputResources applies the control-plane admission rules to the Kubernetes output
// resources of id before any of them is applied. Secrets are namespace-scoped, so only objects in
// the control-plane namespace are checked: one that is or references a control-plane Secret
// violates control-plane-secret, and any other violates control-plane-namespace. This mirrors the
// chart's radius-control-plane-isolation admission policy.
func admitOutputResources(ctx context.Context, opts authz.Options, id resources.ID, outputs []rpv1.OutputResource) error {
	mode := opts.EffectiveMode()
	if mode == authz.ModeOff {
		return nil
	}

	// Report the most specific rule across all outputs so the result doesn't depend on output order.
	rule := ""
	for _, output := range outputs {
		if output.CreateResource == nil || output.GetResourceType().Provider != resourcemodel.ProviderKubernetes {
			continue
		}
		obj, ok := output.CreateResource.Data.(metav1.Object)
		if !ok || !opts.IsControlPlaneNamespace(obj.GetNamespace()) {
			continue
		}
		if referencesControlPlaneSecret(obj, opts) {
			rule = authz.RuleControlPlaneSecret
			break
		}
		rule = authz.RuleControlPlaneNamespace
	}
	if rule == "" {
		return nil
	}

	return authz.Apply(ctx, mode, authz.Decision{
		Code:   v1.CodeAdmissionPolicyDenied,
		Action: id.Type() + "/write",
		Target: id.String(),
		Rule:   rule,
	})
}

// referencesControlPlaneSecret reports whether obj is a control-plane Secret or a workload whose
// pod spec references one.
func referencesControlPlaneSecret(obj metav1.Object, opts authz.Options) bool {
	var spec *corev1.PodSpec
	switch o := obj.(type) {
	case *corev1.Secret:
		return opts.IsControlPlaneSecret(o.Name)
	case *corev1.Pod:
		spec = &o.Spec
	case *appsv1.Deployment:
		spec = &o.Spec.Template.Spec
	default:
		return false
	}

	for _, v := range spec.Volumes {
		if v.Secret != nil && opts.IsControlPlaneSecret(v.Secret.SecretName) {
			return true
		}
		if v.Projected != nil {
			for _, s := range v.Projected.Sources {
				if s.Secret != nil && opts.IsControlPlaneSecret(s.Secret.Name) {
					return true
				}
			}
		}
	}
	for _, r := range spec.ImagePullSecrets {
		if opts.IsControlPlaneSecret(r.Name) {
			return true
		}
	}
	for _, c := range append(append([]corev1.Container{}, spec.InitContainers...), spec.Containers...) {
		for _, e := range c.EnvFrom {
			if e.SecretRef != nil && opts.IsControlPlaneSecret(e.SecretRef.Name) {
				return true
			}
		}
		for _, e := range c.Env {
			if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil && opts.IsControlPlaneSecret(e.ValueFrom.SecretKeyRef.Name) {
				return true
			}
		}
	}

	return false
}
