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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/spf13/pflag"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
)

// CertificateOptions selects a dedicated signing CA without passing private material through Helm.
type CertificateOptions struct {
	CACert     string
	CAKey      string
	CASecret   string
	CertIssuer string
}

// AddFlags registers the certificate options shared by install and upgrade.
func (o *CertificateOptions) AddFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.CACert, "ca-cert", "", "Path to a dedicated signing CA PEM (requires --ca-key; private material is never stored in Helm)")
	flags.StringVar(&o.CAKey, "ca-key", "", "Path to the signing CA private key (requires --ca-cert; imports a retained, create-only Kubernetes Secret)")
	flags.StringVar(&o.CASecret, "ca-secret", "", "Existing operator-owned TLS signing Secret in radius-system; never adopted or overwritten")
	flags.StringVar(&o.CertIssuer, "cert-issuer", "", "External cert-manager issuer (currently rejected until effective request-policy protection can be verified)")
}

// Validate checks flag combinations before contacting the cluster.
func (o CertificateOptions) Validate() error {
	if (o.CACert == "") != (o.CAKey == "") {
		return fmt.Errorf("--ca-cert and --ca-key must be supplied together")
	}
	count := 0
	for _, present := range []bool{o.CACert != "", o.CASecret != "", o.CertIssuer != ""} {
		if present {
			count++
		}
	}
	if count > 1 {
		return fmt.Errorf("--ca-cert/--ca-key, --ca-secret and --cert-issuer are mutually exclusive")
	}
	if o.CertIssuer != "" {
		return issuerPolicyError()
	}
	if o.CASecret != "" {
		return validateCASecretName(o.CASecret)
	}
	return nil
}

// ValidateFlags also rejects explicitly empty certificate flags rather than treating them as omitted.
func (o CertificateOptions) ValidateFlags(flags *pflag.FlagSet) error {
	for _, name := range []string{"ca-cert", "ca-key", "ca-secret", "cert-issuer"} {
		flag := flags.Lookup(name)
		if flag != nil && flag.Changed && flag.Value.String() == "" {
			return fmt.Errorf("--%s must not be empty", name)
		}
	}
	return o.Validate()
}

// Specified reports whether certificate selection was requested.
func (o CertificateOptions) Specified() bool {
	return o.CACert != "" || o.CAKey != "" || o.CASecret != "" || o.CertIssuer != ""
}

func issuerPolicyError() error {
	return fmt.Errorf("external certificate issuers are disabled: verify a running approver-policy controller, disabled default automatic approval, and effective identity- and issuer-scoped request policy; use a dedicated --ca-secret or --ca-cert/--ca-key instead")
}

func validateCASecretName(name string) error {
	if len(validation.IsDNS1123Subdomain(name)) != 0 {
		return fmt.Errorf("CA Secret must have a valid Kubernetes Secret name")
	}
	switch name {
	case "radius-ca", "ucp-cert", "ucp-mtls", "applications-rp-mtls", "dynamic-rp-mtls", "controller-mtls", "bicep-de-mtls":
		return fmt.Errorf("CA Secret must not name a Radius-managed certificate Secret")
	}
	return nil
}

// validateSigningCA accepts a signing CA followed by an optional, ordered issuance parent chain.
// Only the first certificate becomes Radius trust, never the organization root or returned leaf ca.crt.
func validateSigningCA(certPEM, keyPEM []byte, now time.Time) (*x509.Certificate, error) {
	var certs []*x509.Certificate
	for rest := bytes.TrimSpace(certPEM); len(rest) > 0; {
		if !bytes.HasPrefix(rest, []byte("-----BEGIN CERTIFICATE-----")) {
			return nil, fmt.Errorf("CA certificate must contain only PEM certificates")
		}
		block, remaining := pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 ||
			bytes.Count(rest[:len(rest)-len(remaining)], []byte("-----BEGIN ")) != 1 {
			return nil, fmt.Errorf("CA certificate contains malformed PEM")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("CA certificate contains invalid X.509 data")
		}
		if !cert.IsCA || !cert.BasicConstraintsValid || cert.KeyUsage&x509.KeyUsageCertSign == 0 {
			return nil, fmt.Errorf("CA certificate requires CA basic constraints and certificate signing key usage")
		}
		if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
			return nil, fmt.Errorf("CA certificate is not currently valid")
		}
		switch key := cert.PublicKey.(type) {
		case *ecdsa.PublicKey:
			if key.Curve != elliptic.P256() && key.Curve != elliptic.P384() {
				return nil, fmt.Errorf("CA ECDSA key must use P-256 or P-384")
			}
		case *rsa.PublicKey:
			if key.N.BitLen() < 2048 {
				return nil, fmt.Errorf("CA RSA key must be at least 2048 bits")
			}
		default:
			return nil, fmt.Errorf("CA key must use ECDSA P-256/P-384 or RSA with at least 2048 bits")
		}
		if len(cert.UnhandledCriticalExtensions) > 0 {
			return nil, fmt.Errorf("CA certificate has unsupported critical extensions")
		}
		certs = append(certs, cert)
		rest = bytes.TrimSpace(remaining)
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("CA certificate PEM is empty")
	}
	for index := 1; index < len(certs); index++ {
		if !bytes.Equal(certs[index-1].RawIssuer, certs[index].RawSubject) {
			return nil, fmt.Errorf("CA certificate chain must contain the signing CA followed by its issuance parents")
		}
		if err := certs[index-1].CheckSignatureFrom(certs[index]); err != nil {
			return nil, fmt.Errorf("CA certificate chain must contain the signing CA followed by its issuance parents")
		}
		// CheckSignatureFrom does not enforce the parent's path length.
		if certs[index].MaxPathLenZero || (certs[index].MaxPathLen > 0 && index > certs[index].MaxPathLen) {
			return nil, fmt.Errorf("CA certificate chain exceeds a parent's path length constraint")
		}
	}
	last := certs[len(certs)-1]
	if bytes.Equal(last.RawIssuer, last.RawSubject) {
		if err := last.CheckSignatureFrom(last); err != nil {
			return nil, fmt.Errorf("self-signed CA certificate has an invalid or unsupported signature")
		}
	}
	keyBlock, rest := pem.Decode(bytes.TrimSpace(keyPEM))
	if keyBlock == nil || len(bytes.TrimSpace(rest)) != 0 || len(keyBlock.Headers) != 0 ||
		bytes.Count(keyPEM, []byte("-----BEGIN ")) != 1 ||
		!bytes.HasPrefix(bytes.TrimSpace(keyPEM), []byte("-----BEGIN "+keyBlock.Type+"-----")) {
		return nil, fmt.Errorf("CA key must contain exactly one unencrypted PEM private key")
	}
	if _, err := tls.X509KeyPair(bytes.TrimSpace(certPEM), bytes.TrimSpace(keyPEM)); err != nil {
		// Parser errors can contain input-derived data. Never forward them for signing material.
		return nil, fmt.Errorf("CA key must be a supported unencrypted private key matching the first CA certificate")
	}
	return certs[0], nil
}

type certificatePlan struct {
	client    kubernetes.Interface
	namespace string
	secret    *corev1.Secret
}

func (i *Impl) certificateClient(kubeContext string) (kubernetes.Interface, error) {
	if i.newCertificateClient != nil {
		return i.newCertificateClient(kubeContext)
	}
	config, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
		ContextName: kubeContext, QPS: kubeutil.DefaultCLIQPS, Burst: kubeutil.DefaultCLIBurst,
	})
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func certificateValues(values map[string]any) map[string]any {
	global, _ := values["global"].(map[string]any)
	rbac, _ := global["rbac"].(map[string]any)
	certificates, _ := rbac["certificates"].(map[string]any)
	return certificates
}

func certificateMode(values map[string]any) string {
	mode, ok := certificateValues(values)["mode"]
	if !ok {
		return "selfSigned"
	}
	text, _ := mode.(string)
	return text
}

func validateCertificateSettings(values map[string]any) error {
	global, _ := values["global"].(map[string]any)
	rbac, _ := global["rbac"].(map[string]any)
	if raw, exists := rbac["certificates"]; exists && raw != nil {
		if _, ok := raw.(map[string]any); !ok {
			return fmt.Errorf("global.rbac.certificates must be a mapping")
		}
	}
	return nil
}

func setCertificateValues(values map[string]any, certificates map[string]any) {
	global, ok := values["global"].(map[string]any)
	if !ok {
		global = map[string]any{}
		values["global"] = global
	}
	rbac, ok := global["rbac"].(map[string]any)
	if !ok {
		rbac = map[string]any{}
		global["rbac"] = rbac
	}
	rbac["certificates"] = certificates
}

// prepareCertificates performs all certificate checks without changing the cluster. Only public
// certificate values are added to current; signing material stays in the returned in-memory plan.
func (i *Impl) prepareCertificates(ctx context.Context, options ClusterOptions, defaults, current map[string]any, kubeContext string, installed bool) (*certificatePlan, error) {
	o := options.Certificates
	if err := o.Validate(); err != nil {
		return nil, err
	}
	stored := options.RadiusReleaseValues
	for _, values := range []map[string]any{defaults, current, stored} {
		if err := validateCertificateSettings(values); err != nil {
			return nil, err
		}
	}
	if options.ResetValues && len(certificateValues(stored)) > 0 {
		return nil, fmt.Errorf("--reset-values cannot discard stored certificate settings; omit it to preserve the signing CA and trust, or use coordinated CA migration")
	}
	if options.ResetValues {
		stored = nil
	}
	merged := valuesutil.CoalesceTables(copyValues(current), copyValues(stored))
	merged = valuesutil.CoalesceTables(merged, copyValues(defaults))
	var certPEM, keyPEM []byte
	var signingCA *x509.Certificate
	secretName := o.CASecret
	if o.CACert != "" || secretName != "" {
		if len(certificateValues(current)) != 0 {
			return nil, fmt.Errorf("certificate flags cannot be combined with global.rbac.certificates overrides")
		}
		if o.CACert != "" {
			var err error
			certPEM, err = os.ReadFile(o.CACert)
			if err != nil {
				return nil, fmt.Errorf("cannot read --ca-cert file")
			}
			keyPEM, err = os.ReadFile(o.CAKey)
			if err != nil {
				return nil, fmt.Errorf("cannot read --ca-key file")
			}
			signingCA, err = validateSigningCA(certPEM, keyPEM, time.Now())
			if err != nil {
				return nil, err
			}
			fingerprint := sha256.Sum256(signingCA.Raw)
			secretName = fmt.Sprintf("radius-imported-ca-%x", fingerprint[:16])
		}
		setCertificateValues(merged, map[string]any{"mode": "caSecret", "caSecret": secretName})
	}
	mode := certificateMode(merged)
	if mode == "issuerRef" {
		return nil, issuerPolicyError()
	}
	if mode != "selfSigned" && mode != "caSecret" {
		return nil, fmt.Errorf("unsupported certificate mode; use selfSigned or caSecret")
	}
	if installed && certificateMode(stored) != mode {
		return nil, fmt.Errorf("certificate mode changes require coordinated CA migration; keep the installed mode")
	}
	settings := certificateValues(merged)
	if mode == "selfSigned" {
		if settings["caSecret"] != nil && settings["caSecret"] != "" || settings["caBundle"] != nil && settings["caBundle"] != "" {
			return nil, fmt.Errorf("CA Secret and bundle require certificate mode caSecret")
		}
		return nil, nil
	}
	authzMode, err := resolveAuthzMode(nil, merged)
	if err != nil {
		return nil, err
	}
	if authzMode == authz.ModeOff {
		return nil, fmt.Errorf("dedicated CA certificates require global.rbac.dryRun=true or global.rbac.enabled=true")
	}
	secretName, ok := settings["caSecret"].(string)
	if !ok || secretName == "" {
		return nil, fmt.Errorf("certificate mode caSecret requires a signing Secret name")
	}
	if err := validateCASecretName(secretName); err != nil {
		return nil, err
	}
	if installed && certificateValues(stored)["caSecret"] != secretName {
		return nil, fmt.Errorf("changing the signing Secret requires coordinated CA migration")
	}
	client, err := i.certificateClient(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate Kubernetes client: %w", err)
	}
	plan := &certificatePlan{client: client, namespace: options.Radius.Namespace}
	existing, err := client.CoreV1().Secrets(plan.namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil && (!apierrors.IsNotFound(err) || o.CACert == "" || installed) {
		return nil, fmt.Errorf("cannot read signing CA Secret %s/%s; check that it exists and that access is permitted", plan.namespace, secretName)
	}
	if err == nil {
		if existing.Type != corev1.SecretTypeTLS {
			return nil, fmt.Errorf("signing CA Secret must have type kubernetes.io/tls")
		}
		if o.CACert != "" && (!bytes.Equal(existing.Data[corev1.TLSCertKey], certPEM) || !bytes.Equal(existing.Data[corev1.TLSPrivateKeyKey], keyPEM)) {
			return nil, fmt.Errorf("imported CA Secret already exists with different content; it will not be overwritten")
		}
		certPEM, keyPEM = existing.Data[corev1.TLSCertKey], existing.Data[corev1.TLSPrivateKeyKey]
	} else {
		plan.secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: plan.namespace},
			Type:       corev1.SecretTypeTLS,
			Data:       map[string][]byte{corev1.TLSCertKey: certPEM, corev1.TLSPrivateKeyKey: keyPEM},
		}
	}
	signingCA, err = validateSigningCA(certPEM, keyPEM, time.Now())
	if err != nil {
		return nil, err
	}
	bundle := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: signingCA.Raw}))
	if installed && certificateValues(stored)["caBundle"] == nil {
		return nil, fmt.Errorf("stored dedicated CA trust is missing; recover the prior public CA bundle before upgrading instead of adopting current Secret contents")
	}
	// Explicit Helm bundles and stored trust must identify exactly this CA, even if PEM whitespace differs.
	for _, values := range []map[string]any{current, stored} {
		if raw, exists := certificateValues(values)["caBundle"]; exists {
			text, ok := raw.(string)
			if !ok || !sameCACertificate(text, signingCA) {
				return nil, fmt.Errorf("CA trust bundle must contain exactly the dedicated signing CA; changing trust requires coordinated CA migration")
			}
		}
	}
	if o.CACert == "" && o.CASecret == "" {
		raw, ok := settings["caBundle"].(string)
		if !ok || !sameCACertificate(raw, signingCA) {
			return nil, fmt.Errorf("CA trust bundle must contain exactly the dedicated signing CA")
		}
	}
	setCertificateValues(current, map[string]any{"mode": "caSecret", "caSecret": secretName, "caBundle": strings.TrimSpace(bundle)})
	if time.Until(signingCA.NotAfter) <= 30*24*time.Hour {
		i.logInfo("Warning: dedicated signing CA expires within 30 days. Arrange coordinated CA rotation before it expires.")
	}
	return plan, nil
}

func sameCACertificate(text string, signingCA *x509.Certificate) bool {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "-----BEGIN CERTIFICATE-----") || strings.Count(trimmed, "-----BEGIN ") != 1 {
		return false
	}
	block, rest := pem.Decode([]byte(trimmed))
	return block != nil && block.Type == "CERTIFICATE" && len(block.Headers) == 0 &&
		len(bytes.TrimSpace(rest)) == 0 && bytes.Equal(block.Bytes, signingCA.Raw)
}

func (i *Impl) applyCertificatePlan(ctx context.Context, plan *certificatePlan) error {
	if plan == nil || plan.secret == nil {
		return nil
	}
	_, err := plan.client.CoreV1().Namespaces().Get(ctx, plan.namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = plan.client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: plan.namespace}}, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			err = nil
		}
	}
	if err != nil {
		return fmt.Errorf("cannot prepare namespace for imported CA Secret; check namespace permissions")
	}
	_, err = plan.client.CoreV1().Secrets(plan.namespace).Create(ctx, plan.secret, metav1.CreateOptions{})
	if err != nil {
		// Never update or adopt an existing object, including one created between preflight and apply.
		return fmt.Errorf("cannot create imported CA Secret %s/%s; no existing Secret was overwritten. Check permissions and retry the same command to validate any retained Secret", plan.namespace, plan.secret.Name)
	}
	i.logInfo("Imported signing CA into Secret %s/%s outside Helm ownership. It is retained on failure and uninstall; retry the same files after a failure. Back up the key and remove the Secret manually only after no issuer uses it.", plan.namespace, plan.secret.Name)
	return nil
}

// CheckRadiusCertificates validates the upgrade's certificate settings without creating Secrets.
func (i *Impl) CheckRadiusCertificates(ctx context.Context, options ClusterOptions, kubeContext string) error {
	chart, _, current, err := prepareRadiusChart(ctx, NewHelmAction(i.Helm), options.Radius, kubeContext)
	if err != nil {
		return err
	}
	_, err = i.prepareCertificates(ctx, options, chart.Values, current, kubeContext, true)
	return err
}
