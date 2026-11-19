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
	"crypto/x509"
	"errors"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// validateBootstrapRenewal uses the persisted bootstrap identity, never an
// identity inferred from the Secret being checked. Version 1 journals retain a
// certificate from which the same identity can be recovered after its expiry.
func validateBootstrapRenewal(secret *corev1.Secret, anchor initialCertificateAnchor, now time.Time) error {
	pin := anchor.Signer
	if pin == nil || secret.Name != pin.Name || secret.UID != pin.UID ||
		secret.ResourceVersion == "" || secret.DeletionTimestamp != nil ||
		secret.Type != corev1.SecretTypeTLS ||
		secret.Annotations[certificateNameAnnotation] != pin.Name ||
		secret.Annotations["cert-manager.io/issuer-name"] != radiusBootstrapIssuerName ||
		secret.Annotations["cert-manager.io/issuer-kind"] != "Issuer" ||
		secret.Annotations["cert-manager.io/issuer-group"] != "cert-manager.io" {
		return errors.New("bootstrap signer identity or ownership changed")
	}
	old, err := parseCertificateBundlePEM(anchor.CA)
	if err != nil || len(old) != 1 {
		return errors.New("bootstrap renewal has no historical anchor")
	}
	current, err := validateSigningCA(secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey], now)
	if err != nil {
		return err
	}
	public, err := validatePublicCA(secret.Data[caCertificateKey], now)
	if err != nil {
		return err
	}
	if !bytes.Equal(public.Raw, current.Raw) || current.CheckSignatureFrom(current) != nil {
		return errors.New("bootstrap renewal must expose only its self-signed dedicated CA")
	}
	if err := validateUCPAPIServerRenewal(current, old); err != nil {
		return err
	}
	if len(anchor.PublicKey) != 0 && (!bytes.Equal(anchor.PublicKey, current.RawSubjectPublicKeyInfo) ||
		!bytes.Equal(anchor.Subject, current.RawSubject)) {
		return errors.New("bootstrap renewal changed its recorded key or subject")
	}
	if !sameCAConstraints(old[0], current) {
		return errors.New("bootstrap renewal changed its CA constraints")
	}
	return nil
}

func sameCAConstraints(a, b *x509.Certificate) bool {
	return a.IsCA == b.IsCA && a.BasicConstraintsValid == b.BasicConstraintsValid &&
		a.KeyUsage == b.KeyUsage && a.MaxPathLen == b.MaxPathLen && a.MaxPathLenZero == b.MaxPathLenZero &&
		reflect.DeepEqual(a.ExtKeyUsage, b.ExtKeyUsage) && reflect.DeepEqual(a.UnknownExtKeyUsage, b.UnknownExtKeyUsage) &&
		a.PermittedDNSDomainsCritical == b.PermittedDNSDomainsCritical &&
		reflect.DeepEqual(a.PermittedDNSDomains, b.PermittedDNSDomains) &&
		reflect.DeepEqual(a.ExcludedDNSDomains, b.ExcludedDNSDomains) &&
		reflect.DeepEqual(a.PermittedIPRanges, b.PermittedIPRanges) &&
		reflect.DeepEqual(a.ExcludedIPRanges, b.ExcludedIPRanges) &&
		reflect.DeepEqual(a.PermittedEmailAddresses, b.PermittedEmailAddresses) &&
		reflect.DeepEqual(a.ExcludedEmailAddresses, b.ExcludedEmailAddresses) &&
		reflect.DeepEqual(a.PermittedURIDomains, b.PermittedURIDomains) &&
		reflect.DeepEqual(a.ExcludedURIDomains, b.ExcludedURIDomains)
}

// Cainjector may retain earlier certificates for the same key during renewal.
// The current certificate must be present, and no unrelated trust is allowed.
func renewedCertificateSet(bundle []byte, current *x509.Certificate) (bool, error) {
	certificates, err := parseCertificateBundlePEM(bundle)
	if err != nil {
		return false, err
	}
	if err := validateUCPAPIServerRenewal(current, certificates); err != nil {
		return false, err
	}
	found := false
	for _, certificate := range certificates {
		if !sameCAConstraints(certificate, current) {
			return false, errors.New("renewal trust contains changed CA constraints")
		}
		found = found || bytes.Equal(certificate.Raw, current.Raw)
	}
	return found, nil
}
