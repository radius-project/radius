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
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// certificateIssuerSnapshot pins configuration without copying an operator's
// arbitrary issuer spec into the public journal. Recovery validates it; it never
// restores or edits an operator-owned issuer.
type certificateIssuerSnapshot struct {
	Reference       certificateIssuerReference `json:"reference"`
	Namespace       string                     `json:"namespace"`
	UID             types.UID                  `json:"uid"`
	ResourceVersion string                     `json:"resourceVersion"`
	SpecSHA256      string                     `json:"specSHA256"`
	SigningSecret   string                     `json:"signingSecret,omitempty"`
}

type certificatePlaneSnapshot struct {
	Mode   string                     `json:"mode"`
	Issuer certificateIssuerSnapshot  `json:"issuer"`
	Signer *certificateObjectPin      `json:"signer,omitempty"`
	Source certificateTrustProjection `json:"source"`
}

func (snapshot certificateIssuerSnapshot) validate(namespace string) error {
	ref := snapshot.Reference
	if ref.Group != "cert-manager.io" || snapshot.UID == "" || snapshot.ResourceVersion == "" ||
		!validCertificateHash(snapshot.SpecSHA256) {
		return errors.New("issuer snapshot requires observed identity and configuration pins")
	}
	if _, err := parseCertificateIssuer(ref.Kind + "/" + ref.Name); err != nil {
		return err
	}
	wantNamespace := namespace
	if ref.Kind == "ClusterIssuer" {
		wantNamespace = ""
	}
	if snapshot.Namespace != wantNamespace {
		return errors.New("issuer snapshot namespace does not match its scope")
	}
	return nil
}

func (s *rotationStore) snapshotCertificateIssuer(ctx context.Context, ref certificateIssuerReference) (certificateIssuerSnapshot, error) {
	snapshot := certificateIssuerSnapshot{Reference: ref, Namespace: s.namespace}
	if ref.Group != "cert-manager.io" {
		return snapshot, errors.New("unsupported issuer snapshot group")
	}
	if _, err := parseCertificateIssuer(ref.Kind + "/" + ref.Name); err != nil {
		return snapshot, err
	}
	resource := rotationIssuerResource
	if ref.Kind == "ClusterIssuer" {
		resource.Resource, snapshot.Namespace = "clusterissuers", ""
	}
	if err := ctx.Err(); err != nil {
		return snapshot, err
	}
	issuer, err := s.dynamic.Resource(resource).Namespace(snapshot.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return snapshot, err
	}
	if issuer.GetDeletionTimestamp() != nil {
		return snapshot, errors.New("cannot snapshot a terminating issuer")
	}
	snapshot.UID, snapshot.ResourceVersion = issuer.GetUID(), issuer.GetResourceVersion()
	snapshot.SpecSHA256, err = rotationIssuerSpecDigest(issuer)
	if err != nil {
		return snapshot, err
	}
	snapshot.SigningSecret, _, err = unstructured.NestedString(issuer.Object, "spec", "ca", "secretName")
	if err != nil {
		return snapshot, err
	}
	if err := snapshot.validate(s.namespace); err != nil {
		return snapshot, err
	}
	return snapshot, ctx.Err()
}

func (s *rotationStore) checkCertificateIssuerSnapshot(ctx context.Context, expected certificateIssuerSnapshot, phase rotationPhase) error {
	if err := expected.validate(s.namespace); err != nil {
		return err
	}
	current, err := s.snapshotCertificateIssuer(ctx, expected.Reference)
	if err != nil {
		return err
	}
	if current.UID != expected.UID || current.SpecSHA256 != expected.SpecSHA256 || current.SigningSecret != expected.SigningSecret {
		return errors.New("pinned issuer identity or configuration changed; out-of-band signer replacement cannot be recovered as a staged transition")
	}
	if phase != rotationComplete && current.ResourceVersion != expected.ResourceVersion {
		return errors.New("pinned issuer revision changed during the active lifecycle")
	}
	return nil
}

func (s *rotationStore) snapshotCertificatePlane(ctx context.Context, name, mode string, ref certificateIssuerReference) (certificatePlaneSnapshot, error) {
	snapshot := certificatePlaneSnapshot{Mode: mode}
	if !slices.Contains([]string{"internal", "aggregation"}, name) ||
		!slices.Contains([]string{"selfSigned", "caSecret", "issuerRef"}, mode) {
		return snapshot, errors.New("unsupported trust plane or certificate mode")
	}
	if err := ctx.Err(); err != nil {
		return snapshot, err
	}
	issuer, err := s.snapshotCertificateIssuer(ctx, ref)
	if err != nil {
		return snapshot, err
	}
	snapshot.Issuer = issuer
	var source metav1.Object
	if name == "aggregation" {
		object, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
		if err != nil {
			return snapshot, err
		}
		if err := publicAggregationSource(object, s.namespace); err != nil {
			return snapshot, err
		}
		source, snapshot.Source.Kind, snapshot.Source.Bundle = object, "Secret", bytes.Clone(object.Data[caCertificateKey])
	} else {
		object, err := s.kube.CoreV1().ConfigMaps(s.namespace).Get(ctx, "radius-mtls-ca", metav1.GetOptions{})
		if err != nil {
			return snapshot, err
		}
		if object.UID == "" || object.ResourceVersion == "" || object.DeletionTimestamp != nil ||
			len(object.OwnerReferences) != 0 || len(object.Data) != 1 || len(object.BinaryData) != 0 ||
			object.Labels[helmManagedByLabel] != helmManagedByValue ||
			object.Annotations[helmReleaseNameAnnotation] != s.release ||
			object.Annotations[helmReleaseNamespaceAnnotation] != s.namespace {
			return snapshot, errors.New("internal snapshot requires a live owned public-only source")
		}
		source, snapshot.Source.Kind, snapshot.Source.Bundle = object, "ConfigMap", []byte(object.Data[caCertificateKey])
	}
	if source.GetAnnotations()["radius.io/certificate-mode"] != mode {
		return snapshot, errors.New("public source mode differs from the selected certificate configuration")
	}
	certificates, err := parseCertificateBundlePEM(snapshot.Source.Bundle)
	if err != nil || len(certificates) != 1 {
		return snapshot, errors.New("previous public source requires exactly one historical dedicated anchor")
	}
	if _, _, err := rotationHistoricalDigest(snapshot.Source.Bundle); err != nil {
		return snapshot, err
	}
	snapshot.Source.certificateObjectPin = certificateObjectPin{Name: source.GetName(), UID: source.GetUID(),
		ResourceVersion: source.GetResourceVersion(), SHA256: fmt.Sprintf("%x", sha256.Sum256(snapshot.Source.Bundle))}
	if mode != "issuerRef" {
		if err := validateCASecretName(issuer.SigningSecret); err != nil {
			return snapshot, err
		}
		secret, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, issuer.SigningSecret, metav1.GetOptions{})
		if err != nil {
			return snapshot, err
		}
		chain, parseErr := parseCertificateBundlePEM(secret.Data[corev1.TLSCertKey])
		if secret.Type != corev1.SecretTypeTLS || secret.UID == "" || secret.ResourceVersion == "" ||
			secret.DeletionTimestamp != nil || parseErr != nil || len(chain) == 0 || !bytes.Equal(chain[0].Raw, certificates[0].Raw) {
			return snapshot, errors.New("previous issuer signing Secret differs from its dedicated public anchor")
		}
		snapshot.Signer = &certificateObjectPin{Name: secret.Name, UID: secret.UID, ResourceVersion: secret.ResourceVersion,
			SHA256: fmt.Sprintf("%x", sha256.Sum256(chain[0].Raw))}
	}
	// Reading this descriptor is not a lock or readiness proof. The caller must
	// recheck both planes and all revisions under the write guard before mutation.
	if err := s.checkCertificateIssuerSnapshot(ctx, issuer, rotationPublish); err != nil {
		return snapshot, err
	}
	return snapshot, ctx.Err()
}

func (s *rotationStore) snapshotCertificateInstallation(ctx context.Context, baseline *certificateTarget) (map[string]certificatePlaneSnapshot, error) {
	snapshots := make(map[string]certificatePlaneSnapshot, len(baseline.Planes))
	for _, name := range []string{"internal", "aggregation"} {
		plane, found := baseline.Planes[name]
		if !found {
			continue
		}
		snapshot, err := s.snapshotCertificatePlane(ctx, name, baseline.Mode, plane.Issuer)
		if err != nil {
			return nil, fmt.Errorf("%s plane: %w", name, err)
		}
		snapshots[name] = snapshot
	}
	for name, expected := range snapshots {
		current, err := s.snapshotCertificatePlane(ctx, name, baseline.Mode, expected.Issuer.Reference)
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(expected, current) {
			return nil, errors.New("certificate configuration changed while snapshotting both trust planes")
		}
	}
	return snapshots, ctx.Err()
}

// checkCertificateLifecycleRecovery validates persisted observations, not process
// adoption or permission to advance a phase. A writer must also hold the Lease.
func (s *rotationStore) checkCertificateLifecycleRecovery(ctx context.Context, journal *corev1.ConfigMap) error {
	state, err := decodeCertificateLifecycle(journal, s.namespace, s.release, s.now())
	if err != nil {
		return err
	}
	if state.APIServiceUID != "" {
		api, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if api.GetUID() != state.APIServiceUID || api.GetDeletionTimestamp() != nil ||
			api.GetLabels()[helmManagedByLabel] != helmManagedByValue ||
			api.GetAnnotations()[helmReleaseNameAnnotation] != s.release ||
			api.GetAnnotations()[helmReleaseNamespaceAnnotation] != s.namespace {
			return errors.New("recovery APIService identity or ownership changed")
		}
	}
	for name, plane := range state.Planes {
		if err := s.checkLifecycleProjection(ctx, journal, state, name, plane); err != nil {
			return err
		}
		if err := s.checkCertificateIssuerSnapshot(ctx, plane.TargetIssuer, state.Phase); err != nil {
			return err
		}
		if state.Phase != rotationComplete {
			if err := s.checkCertificateIssuerSnapshot(ctx, plane.Previous.Issuer, state.Phase); err != nil {
				return err
			}
		}
		if state.Phase != rotationRetire && state.Phase != rotationComplete {
			if err := s.checkCertificateSignerSnapshot(ctx, plane.Previous.Signer, state.Phase); err != nil {
				return err
			}
		}
		if err := s.checkCertificateSignerSnapshot(ctx, plane.Signer, state.Phase); err != nil {
			return err
		}
	}
	current, err := s.kube.CoreV1().ConfigMaps(s.namespace).Get(ctx, journal.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if current.UID != journal.UID || current.ResourceVersion != journal.ResourceVersion ||
		!reflect.DeepEqual(current.Data, journal.Data) {
		return errors.New("CA lifecycle journal changed during recovery validation")
	}
	return ctx.Err()
}

func (s *rotationStore) checkCertificateSignerSnapshot(ctx context.Context, pin *certificateObjectPin, phase rotationPhase) error {
	if pin == nil {
		return nil
	}
	secret, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, pin.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if secret.Type != corev1.SecretTypeTLS || secret.UID != pin.UID ||
		secret.DeletionTimestamp != nil || secret.ResourceVersion == "" ||
		phase != rotationComplete && secret.ResourceVersion != pin.ResourceVersion {
		return errors.New("signing Secret identity or revision changed")
	}
	certificate, err := validateSigningCA(secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey], s.now())
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(certificate.Raw)) != pin.SHA256 {
		return errors.New("signing Secret changed its dedicated anchor")
	}
	return ctx.Err()
}

func (plane certificateLifecyclePlane) validateSnapshots(namespace, mode string) error {
	before := plane.Previous
	if before == nil || !slices.Contains([]string{"selfSigned", "caSecret", "issuerRef"}, before.Mode) {
		return errors.New("CA lifecycle requires its previous plane configuration snapshot")
	}
	if err := before.Issuer.validate(namespace); err != nil {
		return err
	}
	if err := plane.TargetIssuer.validate(namespace); err != nil {
		return err
	}
	if plane.TargetIssuer.Reference != plane.Issuer {
		return errors.New("target issuer differs from its configuration snapshot")
	}
	if mode != "issuerRef" && (plane.Signer == nil || plane.TargetIssuer.SigningSecret != plane.Signer.Name) {
		return errors.New("target issuer configuration does not select the pinned local signer")
	}
	if before.Source.Kind != plane.Source.Kind || before.Source.Name != plane.Source.Name ||
		before.Source.UID == "" || before.Source.UID != plane.Source.UID || before.Source.ResourceVersion == "" ||
		before.Source.SHA256 != fmt.Sprintf("%x", sha256.Sum256(before.Source.Bundle)) {
		return errors.New("previous public source snapshot must preserve the stable source identity and exact contents")
	}
	old, err := parseCertificateBundlePEM(before.Source.Bundle)
	if err != nil || len(old) != 1 {
		return errors.New("previous public source must contain exactly the historical dedicated anchor")
	}
	selected, err := parseCertificateBundlePEM(plane.OldCA)
	if err != nil || len(selected) != 1 || !bytes.Equal(old[0].Raw, selected[0].Raw) {
		return errors.New("previous public source differs from the selected historical anchor")
	}
	if before.Mode == "issuerRef" {
		if before.Signer != nil {
			return errors.New("external previous issuer must not claim a local signing Secret")
		}
	} else if before.Signer == nil || before.Signer.Name == "" || before.Signer.UID == "" ||
		before.Signer.ResourceVersion == "" || before.Signer.SHA256 != fmt.Sprintf("%x", sha256.Sum256(old[0].Raw)) ||
		before.Issuer.SigningSecret != before.Signer.Name {
		return errors.New("previous local signer requires its observed identity and historical DER pin")
	}
	if plane.Affected && mode == "issuerRef" && before.Issuer.Reference == plane.TargetIssuer.Reference {
		return errors.New("issuerRef CA replacement requires a distinct staged issuer; same-name out-of-band replacement cannot honor trust-before-issuance")
	}
	if !plane.Affected && (before.Mode != mode || before.Issuer.Reference != plane.TargetIssuer.Reference ||
		before.Issuer.UID != plane.TargetIssuer.UID || before.Issuer.SpecSHA256 != plane.TargetIssuer.SpecSHA256 ||
		!reflect.DeepEqual(before.Signer, plane.Signer) || !bytes.Equal(before.Source.Bundle, plane.Source.Bundle)) {
		return errors.New("unaffected plane must retain its issuer configuration and exact public source")
	}
	return nil
}
