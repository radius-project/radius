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
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/radius-project/radius/pkg/authz/mtls"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const rotationTransaction = "radius.io/ca-rotation"

var (
	rotationCertificateResource = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}
	rotationIssuerResource      = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "issuers"}
)

func (s *rotationStore) plannedPhase(ctx context.Context, transaction string, phases ...rotationPhase) (rotationState, error) {
	_, state, err := s.read(ctx)
	if err != nil {
		return rotationState{}, err
	}
	allowed := slices.Contains(phases, state.Phase) || state.Recovery && state.Phase == rotationRetire && slices.Contains(phases, rotationIssue)
	if state.ID != transaction || state.Plan == nil || !allowed {
		return rotationState{}, errors.New("CA rotation transaction, resource plan, or phase changed; reread before retrying")
	}
	return state, nil
}

func (s *rotationStore) checkPlannedSigner(ctx context.Context, state rotationState) error {
	secret, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, state.Plan.SigningSecret, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("cannot read the planned CA signing Secret: %w", err)
	}
	if secret.UID != state.Plan.SigningUID || secret.Type != corev1.SecretTypeTLS || secret.DeletionTimestamp != nil {
		return errors.New("planned CA signing Secret was replaced, is terminating, or has the wrong type")
	}
	ca, err := validateSigningCA(secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey], s.now())
	if err != nil {
		return err
	}
	if !sameCACertificate(string(state.NewCA), ca) {
		return errors.New("planned signing CA changed; operator signing Secrets are never overwritten by rotation")
	}
	return nil
}

func (s *rotationStore) ensureRotationIssuer(ctx context.Context, transaction string) error {
	state, err := s.plannedPhase(ctx, transaction, rotationIssue)
	if err != nil {
		return err
	}
	if err := s.checkPlannedSigner(ctx, state); err != nil {
		return err
	}
	client := s.dynamic.Resource(rotationIssuerResource).Namespace(s.namespace)
	issuer, err := client.Get(ctx, state.Plan.Issuer, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		issuer, err = client.Create(ctx, &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "cert-manager.io/v1", "kind": "Issuer",
			"metadata": map[string]any{"name": state.Plan.Issuer, "namespace": s.namespace,
				"labels": map[string]any{rotationOwner: s.release, rotationTransaction: state.ID}},
			"spec": map[string]any{"ca": map[string]any{"secretName": state.Plan.SigningSecret}},
		}}, metav1.CreateOptions{FieldManager: FieldManager})
		if apierrors.IsAlreadyExists(err) {
			issuer, err = client.Get(ctx, state.Plan.Issuer, metav1.GetOptions{})
		}
	}
	if err != nil {
		return fmt.Errorf("cannot prepare the rotation's dedicated issuer: %w", err)
	}
	secretName, _, err := unstructured.NestedString(issuer.Object, "spec", "ca", "secretName")
	spec, _, specErr := unstructured.NestedMap(issuer.Object, "spec")
	if err != nil || specErr != nil || len(spec) != 1 || secretName != state.Plan.SigningSecret ||
		issuer.GetDeletionTimestamp() != nil || issuer.GetLabels()[rotationOwner] != s.release ||
		issuer.GetLabels()[rotationTransaction] != state.ID || len(issuer.GetOwnerReferences()) != 0 ||
		issuer.GetAnnotations()["meta.helm.sh/release-name"] != "" {
		return errors.New("rotation issuer has unexpected ownership or configuration; it was not adopted or overwritten")
	}
	return nil
}

func rotationOwned(object metav1.Object, namespace, release string) bool {
	return object.GetUID() != "" && object.GetResourceVersion() != "" && object.GetDeletionTimestamp() == nil &&
		object.GetAnnotations()["meta.helm.sh/release-name"] == release &&
		object.GetAnnotations()["meta.helm.sh/release-namespace"] == namespace
}

func checkRotationObjectRevision(object metav1.Object, state rotationState) error {
	revision := object.GetAnnotations()[rotationRevision]
	if revision == "" || rotationPreviousRevision(revision, state) {
		return nil
	}
	for _, phase := range rotationPhases[:slices.Index(rotationPhases, state.Phase)+1] {
		if revision == state.ID+"/"+string(phase) {
			return nil
		}
	}
	return errors.New("resource has a later or unrelated CA rotation revision")
}

// switchRotationCertificate changes issuerRef rather than mutating an issuer's
// signing Secret. cert-manager sees a new Certificate generation and reissues it.
func (s *rotationStore) switchRotationCertificate(ctx context.Context, transaction string, workload rotationWorkload) error {
	client := s.dynamic.Resource(rotationCertificateResource).Namespace(s.namespace)
	certificate, err := client.Get(ctx, workload.Name+"-cert", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("cannot inspect component Certificate: %w", err)
	}
	state, err := s.plannedPhase(ctx, transaction, rotationIssue)
	if err != nil {
		return err
	}
	if !slices.Contains(state.Plan.Consumers, workload) || certificate.GetUID() != workload.CertificateUID ||
		!rotationOwned(certificate, s.namespace, s.release) {
		return errors.New("component Certificate identity or ownership changed")
	}
	if err := checkRotationObjectRevision(certificate, state); err != nil {
		return err
	}
	issuer, _, err := unstructured.NestedString(certificate.Object, "spec", "issuerRef", "name")
	secret, _, secretErr := unstructured.NestedString(certificate.Object, "spec", "secretName")
	if err != nil || secretErr != nil || secret != workload.Name+"-mtls" ||
		(issuer != workload.OldIssuer && issuer != state.Plan.Issuer) {
		return errors.New("component Certificate was modified outside the rotation")
	}
	if err := s.checkPlannedSigner(ctx, state); err != nil {
		return err
	}
	if err := unstructured.SetNestedMap(certificate.Object, map[string]any{
		"name": state.Plan.Issuer, "kind": "Issuer", "group": "cert-manager.io",
	}, "spec", "issuerRef"); err != nil {
		return err
	}
	annotations := certificate.GetAnnotations()
	annotations[rotationRevision] = state.revision()
	certificate.SetAnnotations(annotations)
	_, err = client.Update(ctx, certificate, metav1.UpdateOptions{FieldManager: FieldManager})
	if err != nil {
		return fmt.Errorf("cannot switch component Certificate issuer; reread before retrying: %w", err)
	}
	return nil
}

func (s *rotationStore) ensureAggregationCertificate(ctx context.Context, transaction string) error {
	state, err := s.plannedPhase(ctx, transaction, rotationIssue)
	if err != nil {
		return err
	}
	client := s.dynamic.Resource(rotationCertificateResource).Namespace(s.namespace)
	desiredSpec := rotationAggregationCertificateSpec(state, s.namespace)
	certificate, err := client.Get(ctx, state.Plan.ServingSecret, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		certificate, err = client.Create(ctx, &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
			"metadata": map[string]any{"name": state.Plan.ServingSecret, "namespace": s.namespace,
				"labels": map[string]any{rotationOwner: s.release, rotationTransaction: state.ID}},
			"spec": desiredSpec,
		}}, metav1.CreateOptions{FieldManager: FieldManager})
		if apierrors.IsAlreadyExists(err) {
			certificate, err = client.Get(ctx, state.Plan.ServingSecret, metav1.GetOptions{})
		}
	}
	if err != nil {
		return fmt.Errorf("cannot prepare aggregation Certificate: %w", err)
	}
	return checkAggregationCertificate(certificate, state, s.namespace)
}

func rotationAggregationCertificateSpec(state rotationState, namespace string) map[string]any {
	return map[string]any{
		"secretName": state.Plan.ServingSecret, "duration": "24h", "renewBefore": "8h",
		"privateKey": map[string]any{"algorithm": "ECDSA", "size": int64(256), "rotationPolicy": "Always"},
		"usages":     []any{"digital signature", "server auth"}, "dnsNames": []any{"ucp." + namespace + ".svc"},
		"issuerRef": map[string]any{"name": state.Plan.Issuer, "kind": "Issuer", "group": "cert-manager.io"},
	}
}

func checkAggregationCertificate(certificate *unstructured.Unstructured, state rotationState, namespace string) error {
	spec, _, err := unstructured.NestedMap(certificate.Object, "spec")
	if err != nil || certificate.GetLabels()[rotationOwner] != state.Release ||
		certificate.GetLabels()[rotationTransaction] != state.ID || certificate.GetDeletionTimestamp() != nil ||
		len(certificate.GetOwnerReferences()) != 0 || certificate.GetAnnotations()["meta.helm.sh/release-name"] != "" {
		return errors.New("aggregation Certificate has unexpected ownership")
	}
	// Compare the fields we declare while allowing cert-manager's API defaults.
	for key, expected := range rotationAggregationCertificateSpec(state, namespace) {
		actual := spec[key]
		if !rotationJSONEqual(actual, expected) {
			return errors.New("aggregation Certificate specification changed; it was not overwritten")
		}
	}
	return nil
}

// rotationLeafDigest verifies the actual issued key pair and chain against the
// selected dedicated CA, not Ready status or the leaf Secret's ca.crt.
func rotationLeafDigest(secret *corev1.Secret, ca []byte, namespace, component string, clientAuth bool, now time.Time) (string, error) {
	if secret == nil || secret.UID == "" || secret.DeletionTimestamp != nil || secret.Type != corev1.SecretTypeTLS ||
		len(secret.Data[corev1.TLSCertKey]) > maxRotationBytes || len(secret.Data[corev1.TLSPrivateKeyKey]) > maxRotationBytes {
		return "", errors.New("issued certificate Secret has invalid identity, type, or size")
	}
	_, roots, err := mtls.TrustBundleDigest(ca, now)
	if err != nil || len(roots) != 1 {
		return "", errors.New("issued leaf verification requires the selected dedicated CA")
	}
	pair, err := tls.X509KeyPair(secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey])
	if err != nil || len(pair.Certificate) == 0 {
		return "", errors.New("issued certificate and private key are not a matching pair")
	}
	certificates := make([]*x509.Certificate, 0, len(pair.Certificate))
	intermediates := x509.NewCertPool()
	for index, der := range pair.Certificate {
		certificate, err := x509.ParseCertificate(der)
		if err != nil {
			return "", errors.New("issued certificate chain is invalid")
		}
		certificates = append(certificates, certificate)
		if index > 0 {
			intermediates.AddCert(certificate)
		}
	}
	leaf := certificates[0]
	identity, err := mtls.PeerIdentity(certificates, namespace)
	if err != nil || identity.Component != component || leaf.IsCA || leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return "", errors.New("issued leaf has the wrong Radius identity or signing constraints")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return "", errors.New("invalid selected dedicated CA")
	}
	usages := []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	if clientAuth {
		usages = append(usages, x509.ExtKeyUsageClientAuth)
	}
	for _, usage := range usages {
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, Intermediates: intermediates, CurrentTime: now,
			DNSName: component + "." + namespace + ".svc", KeyUsages: []x509.ExtKeyUsage{usage}}); err != nil {
			return "", errors.New("issued leaf does not verify against the selected CA, identity, validity, and usages")
		}
	}
	digest := sha256.Sum256(leaf.Raw)
	return hex.EncodeToString(digest[:]), nil
}

func rotationJSONEqual(left, right any) bool {
	a, err := json.Marshal(left)
	if err != nil {
		return false
	}
	b, err := json.Marshal(right)
	return err == nil && bytes.Equal(a, b)
}
