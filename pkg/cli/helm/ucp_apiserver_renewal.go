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
	"crypto/x509"
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
)

// validateUCPAPIServerRenewal permits reissuance, not a change of trust identity.
// Historical pins may have expired: only their subject and public key are compared.
func validateUCPAPIServerRenewal(ca *x509.Certificate, pins []*x509.Certificate) error {
	for _, pin := range pins {
		if !bytes.Equal(ca.RawSubjectPublicKeyInfo, pin.RawSubjectPublicKeyInfo) ||
			!bytes.Equal(ca.RawSubject, pin.RawSubject) {
			return fmt.Errorf("selfSigned aggregation CA key or subject changed; preserve the existing trust and use the certificate lifecycle for CA rotation")
		}
	}
	return nil
}

func validateUCPAPIServerAPIAnchors(ca *x509.Certificate, api *unstructured.Unstructured) error {
	if ca == nil {
		return nil
	}
	encoded, _, err := unstructured.NestedString(api.Object, "spec", "caBundle")
	if err != nil {
		return fmt.Errorf("invalid APIService caBundle: %w", err)
	}
	if encoded == "" {
		return nil
	}
	bundle, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("invalid APIService caBundle: %w", err)
	}
	anchors, err := parseCertificateBundlePEM(bundle)
	if err != nil {
		return fmt.Errorf("invalid APIService caBundle: %w", err)
	}
	for _, anchor := range anchors {
		// The initial Off-to-On transition also trusts the unrelated ucp-ca.
		// A prior managed root pins both subject and SPKI, including after expiry.
		if anchor.Subject.CommonName == ucpAPIServerCASecretName {
			if err := validateUCPAPIServerRenewal(ca, []*x509.Certificate{anchor}); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateUCPAPIServerCAOwnership(secret *corev1.Secret) error {
	a := secret.Annotations
	if secret.Type != corev1.SecretTypeTLS ||
		a[certificateNameAnnotation] != ucpAPIServerCASecretName ||
		a["cert-manager.io/issuer-name"] != RadiusBootstrapIssuerName ||
		a["cert-manager.io/issuer-kind"] != "Issuer" ||
		a["cert-manager.io/issuer-group"] != "cert-manager.io" ||
		len(secret.Data[corev1.TLSCertKey]) == 0 || len(secret.Data[corev1.TLSPrivateKeyKey]) == 0 ||
		len(secret.Data[caCertificateKey]) == 0 {
		return fmt.Errorf("selfSigned aggregation Secret %s/%s is foreign or has an invalid profile: require a TLS Secret for Certificate %s issued by cert-manager.io/Issuer %s with tls.crt, tls.key and ca.crt; preserve it and repair ownership before retrying",
			secret.Namespace, secret.Name, ucpAPIServerCASecretName, RadiusBootstrapIssuerName)
	}
	return nil
}

// retainedUCPAPIServerTrust recognizes the public copy installed by A8, not the
// transient overlap used by older rad commands. Keep it as a continuity pin.
func retainedUCPAPIServerTrust(ctx context.Context, client kubernetes.Interface, namespace string) (*corev1.Secret, error) {
	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read retained aggregation trust: %w", err)
	}
	mode, recorded := secret.Annotations["radius.io/certificate-mode"]
	if !recorded {
		return nil, nil
	}
	if mode != "selfSigned" {
		return nil, fmt.Errorf("retained aggregation trust Secret %s/%s records certificate mode %q; preserve it and use the certificate lifecycle before switching to selfSigned", namespace, secret.Name, mode)
	}
	if secret.Type != corev1.SecretTypeOpaque ||
		secret.Labels["app.kubernetes.io/managed-by"] != "Helm" ||
		secret.Annotations["helm.sh/resource-policy"] != "keep" ||
		secret.Annotations["meta.helm.sh/release-name"] == "" ||
		secret.Annotations["meta.helm.sh/release-namespace"] != namespace ||
		secret.Annotations[allowDirectInjectionAnnotation] != "true" ||
		len(secret.Data) != 1 || len(secret.Data[caCertificateKey]) == 0 {
		return nil, fmt.Errorf("retained aggregation trust Secret %s/%s is foreign or invalid; preserve it and resolve ownership or use the certificate lifecycle before retrying", namespace, secret.Name)
	}
	return secret, nil
}
