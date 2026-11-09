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

package helm

import (
	"context"
	"errors"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func providerComponentCertificateSpec(ca rotationState, namespace, component string) map[string]any {
	return map[string]any{
		"secretName": component + "-mtls", "duration": "24h", "renewBefore": "8h",
		"privateKey": map[string]any{"algorithm": "ECDSA", "size": int64(256), "rotationPolicy": "Always"},
		"usages":     []any{"digital signature", "server auth", "client auth"}, "dnsNames": []any{component + "." + namespace + ".svc"},
		"issuerRef": ca.Plan.issuerReference(),
	}
}

func validateProviderComponentCertificate(object *unstructured.Unstructured, ca rotationState, namespace, component string, allowOldIssuer bool) error {
	if object.GetName() != component+"-cert" || object.GetNamespace() != namespace ||
		!rotationOwned(object, namespace, radiusReleaseName) || len(object.GetOwnerReferences()) != 0 {
		return errors.New("provider component Certificate has unexpected identity or ownership")
	}
	spec, found, err := unstructured.NestedMap(object.Object, "spec")
	if err != nil || !found {
		return errors.New("provider component Certificate has no valid specification")
	}
	expected := providerComponentCertificateSpec(ca, namespace, component)
	for key, actual := range spec {
		switch key {
		case "revisionHistoryLimit":
			// A server default does not change the requested identity or issuer.
			if number, ok := actual.(int64); !ok || number < 1 {
				return errors.New("provider component Certificate has invalid history retention")
			}
		case "privateKey":
			keySpec, ok := actual.(map[string]any)
			if !ok {
				return errors.New("provider component Certificate has invalid key parameters")
			}
			if encoding, exists := keySpec["encoding"]; exists {
				if encoding != "PKCS1" && encoding != "PKCS8" {
					return errors.New("provider component Certificate has unsupported key encoding")
				}
				delete(keySpec, "encoding")
			}
			if !rotationJSONEqual(keySpec, expected[key]) {
				return errors.New("provider component Certificate key parameters changed")
			}
		case "issuerRef":
			issuer, ok := actual.(map[string]any)
			if !ok || len(issuer) != 3 || issuer["group"] != "cert-manager.io" ||
				issuer["kind"] != "Issuer" && issuer["kind"] != "ClusterIssuer" {
				return errors.New("provider component Certificate has an invalid issuer")
			}
			name, ok := issuer["name"].(string)
			if !ok || name == "" || !allowOldIssuer && !rotationJSONEqual(actual, expected[key]) {
				return errors.New("provider component Certificate does not use the unchanged selected CA")
			}
		default:
			want, declared := expected[key]
			if !declared || !rotationJSONEqual(actual, want) {
				return errors.New("provider component Certificate identity or usages changed")
			}
		}
	}
	for key := range expected {
		if _, found := spec[key]; !found {
			return errors.New("provider component Certificate is missing a required field")
		}
	}
	return nil
}

// The external source has no component consumers, so canonical dormant
// Certificates may be moved to the unchanged aggregation CA before quiescence.
// The aggregate Certificate, issuer, signing Secret and CA journal are never written.
func (s *rotationStore) prepareProviderCertificates(ctx context.Context, ca rotationState,
	beforeWrite func(context.Context) error, readOnly bool) error {
	if ca.Plan == nil || ca.Phase != rotationComplete {
		return errors.New("provider credential preparation requires completed aggregation")
	}
	client := s.dynamic.Resource(rotationCertificateResource).Namespace(s.namespace)
	type change struct {
		object *unstructured.Unstructured
		create bool
	}
	var changes []change
	for _, component := range rotationComponents {
		current, err := client.Get(ctx, component+"-cert", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			if _, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, component+"-mtls", metav1.GetOptions{}); err == nil {
				return errors.New("an existing component Secret has no owned Certificate; inspect it before requesting a new provider credential")
			} else if !apierrors.IsNotFound(err) {
				return err
			}
			changes = append(changes, change{create: true, object: &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
				"metadata": map[string]any{"name": component + "-cert", "namespace": s.namespace,
					"labels": map[string]any{"app.kubernetes.io/managed-by": "Helm", "app.kubernetes.io/name": component, "app.kubernetes.io/part-of": "radius"},
					"annotations": map[string]any{"meta.helm.sh/release-name": s.release,
						"meta.helm.sh/release-namespace": s.namespace, rotationRevision: ca.revision()}},
				"spec": providerComponentCertificateSpec(ca, s.namespace, component),
			}}})
			continue
		}
		if err != nil {
			return err
		}
		if err := validateProviderComponentCertificate(current, ca, s.namespace, component, true); err != nil {
			return err
		}
		issuer, _, err := unstructured.NestedMap(current.Object, "spec", "issuerRef")
		if err != nil {
			return err
		}
		if !rotationJSONEqual(issuer, ca.Plan.issuerReference()) {
			next := current.DeepCopy()
			if err := unstructured.SetNestedMap(next.Object, ca.Plan.issuerReference(), "spec", "issuerRef"); err != nil {
				return err
			}
			annotations := next.GetAnnotations()
			annotations[rotationRevision] = ca.revision()
			next.SetAnnotations(annotations)
			changes = append(changes, change{object: next})
		}
	}
	if readOnly {
		return nil
	}
	if beforeWrite == nil {
		return errors.New("provider credential preparation requires the shared operation guard")
	}
	for _, change := range changes {
		if err := beforeWrite(ctx); err != nil {
			return err
		}
		if change.create {
			if _, err := client.Create(ctx, change.object, metav1.CreateOptions{FieldManager: FieldManager}); err != nil {
				return fmt.Errorf("cannot create provider component Certificate; retry the same transition: %w", err)
			}
		} else if _, err := client.Update(ctx, change.object, metav1.UpdateOptions{FieldManager: FieldManager}); err != nil {
			return fmt.Errorf("cannot prepare provider component Certificate; retry the same transition: %w", err)
		}
	}
	return nil
}

func (s *rotationStore) providerSourceSecrets(ctx context.Context, ca rotationState) ([]providerSourceSecret, error) {
	var pins []providerSourceSecret
	for _, component := range rotationComponents {
		certificate, err := s.dynamic.Resource(rotationCertificateResource).Namespace(s.namespace).Get(ctx, component+"-cert", metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if err := validateProviderComponentCertificate(certificate, ca, s.namespace, component, false); err != nil {
			return nil, err
		}
		secret, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, component+"-mtls", metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if secret.UID == "" || secret.DeletionTimestamp != nil {
			return nil, errors.New("provider source credential identity is unavailable")
		}
		pins = append(pins, providerSourceSecret{Name: secret.Name, UID: secret.UID})
	}
	return pins, nil
}

// cert-manager does not necessarily garbage-collect the private key when Helm
// removes a Certificate. Retire only the exact old component Secrets, after all
// target workloads are verified and the source Certificates have been removed.
func (s *rotationStore) retireProviderSecrets(ctx context.Context, plan providerTransitionPlan, fence func(context.Context) error) error {
	if plan.Source != "radius" || plan.Target != "external" || len(plan.SourceSecrets) != len(rotationComponents) || fence == nil {
		return errors.New("provider credential retirement requires its complete source identity inventory and write fence")
	}
	if err := (providerTransitionState{Version: 1, Plan: plan, Phase: providerRecording}).validate(); err != nil {
		return err
	}
	for _, component := range rotationComponents {
		if _, err := s.dynamic.Resource(rotationCertificateResource).Namespace(s.namespace).Get(ctx, component+"-cert", metav1.GetOptions{}); err == nil {
			return errors.New("source component Certificates remain; finish their Helm removal before retiring private keys")
		} else if !apierrors.IsNotFound(err) {
			return err
		}
	}
	var retiring []*corev1.Secret
	for _, pin := range plan.SourceSecrets {
		secret, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, pin.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		if secret.UID != pin.UID || secret.ResourceVersion == "" {
			return errors.New("source component Secret was replaced; it will not be deleted")
		}
		retiring = append(retiring, secret)
	}
	for _, secret := range retiring {
		if err := fence(ctx); err != nil {
			return err
		}
		err := s.kube.CoreV1().Secrets(s.namespace).Delete(ctx, secret.Name, metav1.DeleteOptions{
			Preconditions: &metav1.Preconditions{UID: &secret.UID, ResourceVersion: &secret.ResourceVersion},
		})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (s *rotationStore) checkProviderCertificates(ctx context.Context, ca rotationState) error {
	for _, component := range rotationComponents {
		certificate, err := s.dynamic.Resource(rotationCertificateResource).Namespace(s.namespace).Get(ctx, component+"-cert", metav1.GetOptions{})
		if err != nil {
			return err
		}
		if err := validateProviderComponentCertificate(certificate, ca, s.namespace, component, false); err != nil {
			return err
		}
		secret, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, component+"-mtls", metav1.GetOptions{})
		if err != nil {
			return err
		}
		if _, err := rotationLeafDigest(secret, ca.NewCA, s.namespace, component, true, s.now()); err != nil {
			return fmt.Errorf("waiting for selected-CA credential for %s: %w", component, err)
		}
	}
	return nil
}

func (s *rotationStore) prepareProviderDiscovery(ctx context.Context, prepared providerPreparedResources,
	beforeWrite func(context.Context) error, readOnly bool) error {
	if len(prepared.discovery) != 2 {
		return errors.New("external provider requires exactly its fixed discovery role and binding")
	}
	objects := slices.Clone(prepared.discovery)
	// Create the role before its binding, independent of manifest order.
	slices.SortFunc(objects, func(a, b *unstructured.Unstructured) int {
		if a.GetKind() == b.GetKind() {
			return 0
		}
		if a.GetKind() == "ClusterRole" {
			return -1
		}
		return 1
	})
	var missing []*unstructured.Unstructured
	seen := map[string]bool{}
	for _, desired := range objects {
		if desired.GetAPIVersion() != "rbac.authorization.k8s.io/v1" ||
			!slices.Contains([]string{"ClusterRole", "ClusterRoleBinding"}, desired.GetKind()) ||
			desired.GetName() != "radius-token-discovery-"+s.namespace || desired.GetNamespace() != "" ||
			seen[desired.GetKind()] {
			return errors.New("provider discovery requires exactly the canonical role and binding")
		}
		seen[desired.GetKind()] = true
		if err := validateProviderDiscovery(desired, s.namespace); err != nil {
			return err
		}
		current, err := s.dynamic.Resource(providerDiscoveryResource(desired)).Get(ctx, desired.GetName(), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			missing = append(missing, desired)
			continue
		}
		if err != nil {
			return err
		}
		if !rotationOwned(current, s.namespace, s.release) || len(current.GetOwnerReferences()) != 0 {
			return errors.New("provider discovery permissions are foreign or terminating; they will not be overwritten")
		}
		if err := validateProviderDiscovery(current, s.namespace); err != nil {
			return err
		}
	}
	if readOnly {
		return nil
	}
	if beforeWrite == nil {
		return errors.New("provider discovery preparation requires the shared operation guard")
	}
	for _, object := range missing {
		desired := object.DeepCopy()
		annotations := desired.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations["meta.helm.sh/release-name"], annotations["meta.helm.sh/release-namespace"] = s.release, s.namespace
		desired.SetAnnotations(annotations)
		labels := desired.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels["app.kubernetes.io/managed-by"] = "Helm"
		desired.SetLabels(labels)
		if err := beforeWrite(ctx); err != nil {
			return err
		}
		if _, err := s.dynamic.Resource(providerDiscoveryResource(desired)).Create(ctx, desired, metav1.CreateOptions{FieldManager: FieldManager}); err != nil {
			return fmt.Errorf("cannot prepare fixed provider discovery permissions; retry the same transition: %w", err)
		}
	}
	return nil
}

func providerDiscoveryResource(object *unstructured.Unstructured) schema.GroupVersionResource {
	resource := "clusterroles"
	if object.GetKind() == "ClusterRoleBinding" {
		resource = "clusterrolebindings"
	}
	return schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: resource}
}
