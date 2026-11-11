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
	"strings"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/resourcemodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (dp *deploymentProcessor) admitOutputResources(ctx context.Context, id resources.ID, outputs []rpv1.OutputResource) error {
	opts := dp.authorization
	if opts.EffectiveMode() == authz.ModeOff {
		return nil
	}
	for _, output := range outputs {
		if output.CreateResource == nil || output.GetResourceType().Provider != resourcemodel.ProviderKubernetes {
			continue
		}
		obj, ok := output.CreateResource.Data.(metav1.Object)
		if !ok {
			continue
		}
		namespace, protected := protectedNamespace(obj, opts)
		if !protected {
			continue
		}
		var err error
		opts, err = kubeutil.DeploymentTargetAuthorization(ctx, opts, dp.controlPlaneClient, dp.k8sClient, namespace)
		if err != nil {
			return err
		}
		break
	}
	return admitOutputResources(ctx, opts, id, outputs)
}

// protectedNamespace returns a control-plane namespace that obj is in or references a Secret in.
func protectedNamespace(obj metav1.Object, opts authz.Options) (string, bool) {
	if ns := admissionNamespace(obj); opts.IsControlPlaneNamespace(ns) {
		return ns, true
	}
	for _, ref := range secretReferences(obj) {
		if opts.IsControlPlaneNamespace(ref.Namespace) {
			return ref.Namespace, true
		}
	}
	return "", false
}

// admitOutputResources applies the control-plane admission rules to the Kubernetes output
// resources of id before any of them is applied. An object in the control-plane namespace, the
// Namespace object itself, or an object that references a Secret in the control-plane namespace
// (such as a Contour HTTPProxy TLS secretName of the form "namespace/name") is checked: one that
// is or references a control-plane Secret violates control-plane-secret, and any other violates
// control-plane-namespace. In-namespace objects mirror the chart's radius-control-plane-isolation
// admission policy; cross-namespace references match authz.Options.AdmitSecretReference.
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
		if !ok {
			continue
		}
		if r := outputRule(obj, opts); r != "" {
			rule = r
			if r == authz.RuleControlPlaneSecret {
				break
			}
		}
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

func admissionNamespace(obj metav1.Object) string {
	if ns, ok := obj.(*corev1.Namespace); ok {
		return ns.Name
	}
	return obj.GetNamespace()
}

// outputRule returns the admission rule obj violates, or "" if it is allowed.
func outputRule(obj metav1.Object, opts authz.Options) string {
	rule := ""
	if opts.IsControlPlaneNamespace(admissionNamespace(obj)) {
		if secret, ok := obj.(*corev1.Secret); ok && opts.IsControlPlaneSecret(secret.Name) {
			return authz.RuleControlPlaneSecret
		}
		rule = authz.RuleControlPlaneNamespace
	}
	for _, ref := range secretReferences(obj) {
		if !opts.IsControlPlaneNamespace(ref.Namespace) {
			continue
		}
		if opts.IsControlPlaneSecret(ref.Name) {
			return authz.RuleControlPlaneSecret
		}
		rule = authz.RuleControlPlaneNamespace
	}
	return rule
}

// secretReferences returns the Secrets obj references: those in a workload's pod spec, which are in
// the workload's namespace, and those in a Contour HTTPProxy, which Contour resolves in another
// namespace when written as "namespace/name".
func secretReferences(obj metav1.Object) []types.NamespacedName {
	switch o := obj.(type) {
	case *corev1.Pod:
		return podSpecSecretReferences(o.Namespace, &o.Spec)
	case *appsv1.Deployment:
		return podSpecSecretReferences(o.Namespace, &o.Spec.Template.Spec)
	case *contourv1.HTTPProxy:
		return httpProxySecretReferences(o)
	default:
		return nil
	}
}

func podSpecSecretReferences(namespace string, spec *corev1.PodSpec) []types.NamespacedName {
	var names []string
	for _, v := range spec.Volumes {
		if v.Secret != nil {
			names = append(names, v.Secret.SecretName)
		}
		if v.Projected != nil {
			for _, s := range v.Projected.Sources {
				if s.Secret != nil {
					names = append(names, s.Secret.Name)
				}
			}
		}
	}
	for _, r := range spec.ImagePullSecrets {
		names = append(names, r.Name)
	}
	for _, c := range append(append([]corev1.Container{}, spec.InitContainers...), spec.Containers...) {
		for _, e := range c.EnvFrom {
			if e.SecretRef != nil {
				names = append(names, e.SecretRef.Name)
			}
		}
		for _, e := range c.Env {
			if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
				names = append(names, e.ValueFrom.SecretKeyRef.Name)
			}
		}
	}

	refs := make([]types.NamespacedName, 0, len(names))
	for _, name := range names {
		refs = append(refs, types.NamespacedName{Namespace: namespace, Name: name})
	}
	return refs
}

func httpProxySecretReferences(proxy *contourv1.HTTPProxy) []types.NamespacedName {
	var values []string
	if vh := proxy.Spec.VirtualHost; vh != nil && vh.TLS != nil {
		values = append(values, vh.TLS.SecretName)
		if cv := vh.TLS.ClientValidation; cv != nil {
			values = append(values, cv.CACertificate, cv.CertificateRevocationList)
		}
	}
	services := []contourv1.Service{}
	for _, route := range proxy.Spec.Routes {
		services = append(services, route.Services...)
	}
	if proxy.Spec.TCPProxy != nil {
		services = append(services, proxy.Spec.TCPProxy.Services...)
	}
	for _, svc := range services {
		if svc.UpstreamValidation != nil {
			values = append(values, svc.UpstreamValidation.CACertificate)
		}
	}

	var refs []types.NamespacedName
	for _, value := range values {
		if value == "" {
			continue
		}
		ref := types.NamespacedName{Namespace: proxy.Namespace, Name: value}
		if namespace, name, ok := strings.Cut(value, "/"); ok {
			ref = types.NamespacedName{Namespace: namespace, Name: name}
		}
		refs = append(refs, ref)
	}
	return refs
}
