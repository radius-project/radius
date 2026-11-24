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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"helm.sh/helm/v4/pkg/chart/common"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/engine"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// certificateTarget contains public desired state only. It does not authorize writes,
// approve a transition, or prove that an external issuer can actually issue a leaf.
type certificateTarget struct {
	mode           authz.Mode
	Mode, Provider string
	TrustDomain    string
	Receivers      []string
	Planes         map[string]certificatePlaneTarget
	Values         map[string]any
	SigningSecret  string
	SigningUID     types.UID
	SigningVersion string
	SigningDERHash string
}

type certificatePlaneTarget struct {
	Issuer certificateIssuerReference
	CA     []byte
	Source certificatePublicSource
	// A selfSigned target remains unresolved until the lifecycle stages its CA.
	Unresolved bool
}

// certificatePublicSource is a planned reference from the shared chart helper.
// Offline target preparation does not establish a live UID or resource version.
type certificatePublicSource struct {
	Kind, Name, Key, Bundle             string
	UID, ResourceVersion                string
	ContentDigest, IssuerDigest, Digest string
	Issuer                              certificateIssuerReference
}

// An external provider must not depend on Radius internal serving keys or anchors,
// even if a chart conditionally adds the volume only on upgrade.
func (t *certificateTarget) checkProviderRender(ctx context.Context, render renderRadiusManifest) error {
	if t.Provider != "external" || t.mode == authz.ModeOff {
		return nil
	}
	manifest, applied, err := render(ctx)
	if err != nil || !applied {
		return err
	}
	decode := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		var obj unstructured.Unstructured
		if err := decode.Decode(&obj.Object); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		path := []string{"spec", "template", "spec", "volumes"}
		switch obj.GetKind() {
		case "Pod":
			path = []string{"spec", "volumes"}
		case "CronJob":
			path = []string{"spec", "jobTemplate", "spec", "template", "spec", "volumes"}
		case "Certificate":
			name, _, err := unstructured.NestedString(obj.Object, "spec", "secretName")
			if err != nil {
				return err
			}
			if strings.HasSuffix(name, "-spiffe") || name == "radius-ca" {
				return fmt.Errorf("external TLS provider cannot issue internal serving material %q", name)
			}
		}
		volumes, _, err := unstructured.NestedSlice(obj.Object, path...)
		if err != nil {
			return err
		}
		for _, raw := range volumes {
			v, _ := raw.(map[string]any)
			if usesInternalSource(v, "secretName") {
				return fmt.Errorf("external TLS provider cannot mount Radius internal certificate material in %s", obj.GetName())
			}
			sources, _, err := unstructured.NestedSlice(v, "projected", "sources")
			if err != nil {
				return err
			}
			for _, source := range sources {
				s, _ := source.(map[string]any)
				if usesInternalSource(s, "name") {
					return fmt.Errorf("external TLS provider cannot project Radius internal certificate material in %s", obj.GetName())
				}
			}
		}
	}
}

func usesInternalSource(source map[string]any, secretField string) bool {
	name, _, _ := unstructured.NestedString(source, "secret", secretField)
	cm, _, _ := unstructured.NestedString(source, "configMap", "name")
	return strings.HasSuffix(name, "-spiffe") || name == "radius-ca" || cm == "radius-mtls-ca"
}

func (i *Impl) prepareSelectedCertificates(ctx context.Context, options ClusterOptions, selected *chart.Chart, current map[string]any, kubeContext string, deployed map[string]any) (*certificateTarget, *certificatePlan, error) {
	target, plan, err := i.prepareCertificateTargetForBaseline(ctx, options, selected, current, kubeContext, deployed)
	if err != nil {
		return nil, nil, err
	}
	if deployed != nil {
		if err := unchangedCertificateSelection(deployed, target.Values); err != nil {
			return nil, nil, fmt.Errorf("the installed chart selects different certificate settings; changing issuers requires coordinated CA migration: %w", err)
		}
		if plan != nil && plan.secret != nil {
			return nil, nil, fmt.Errorf("an installed signing Secret is missing; recover it or use coordinated CA migration")
		}
	}
	return target, plan, nil
}

// prepareCertificateTarget runs before the ordinary anti-switch guard so the lifecycle
// can compare a validated target with its journal. The import handle never enters target.
func (i *Impl) prepareCertificateTarget(ctx context.Context, options ClusterOptions, selected *chart.Chart, current map[string]any, kubeContext string) (*certificateTarget, *certificatePlan, error) {
	return i.prepareCertificateTargetForBaseline(ctx, options, selected, current, kubeContext, nil)
}

func (i *Impl) prepareCertificateTargetForBaseline(ctx context.Context, options ClusterOptions, selected *chart.Chart, current map[string]any, kubeContext string, deployed map[string]any) (*certificateTarget, *certificatePlan, error) {
	plan, err := i.prepareCertificateSelection(ctx, options, selected, current, kubeContext, deployed)
	if err != nil {
		return nil, nil, err
	}
	if plan != nil && plan.secret != nil {
		plan.requiresLease = options.RadiusInstalled || options.RadiusReleaseValues != nil
	}
	effective, settings, err := selectedCertificateValues(options, selected, current)
	if err != nil {
		return nil, nil, err
	}
	target, err := certificateProfilePlan(selected, effective, options.Radius.Namespace, settings)
	if err != nil {
		return nil, nil, err
	}
	if target.Mode == "selfSigned" {
		return target, plan, nil
	}
	ca, err := validatePublicCA([]byte(stringValue(certificateValues(effective)["caBundle"])), time.Now())
	if err != nil {
		return nil, nil, err
	}
	chain := []*x509.Certificate{ca}
	if plan != nil {
		chain = plan.chain
		if plan.signingSecret != nil {
			target.SigningUID = plan.signingSecret.UID
			target.SigningVersion = plan.signingSecret.ResourceVersion
		}
	}
	if err := validateCAProfiles(chain, target, options.Radius.Namespace, time.Now()); err != nil {
		return nil, nil, err
	}
	target.SigningSecret = stringValue(certificateValues(effective)["caSecret"])
	target.SigningDERHash = fmt.Sprintf("%x", sha256.Sum256(ca.Raw))
	for plane, selection := range target.Planes {
		if !sameCACertificate(selection.Source.Bundle, ca) {
			return nil, nil, fmt.Errorf("selected chart public %s source does not match the dedicated signing CA", plane)
		}
		selection.CA = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})
		selection.Unresolved = false
		target.Planes[plane] = selection
	}
	return target, plan, nil
}

// certificateProfilePlan consumes the selected chart's shared issuer/receiver helpers,
// rather than maintaining a second component inventory or policy evaluator in Go.
func certificateProfilePlan(selected *chart.Chart, effective map[string]any, namespace string, mode authz.Mode) (*certificateTarget, error) {
	target := &certificateTarget{
		mode: mode,
		Mode: certificateMode(effective), Values: copyValues(effective),
		Planes: map[string]certificatePlaneTarget{},
	}
	if mode == authz.ModeOff || selfSignedOnly(selected.Values) {
		target.Provider = stringValue(nestedValue(effective, "global", "rbac", "tls", "provider"))
		return target, nil
	}
	if selected.Metadata == nil {
		return nil, fmt.Errorf("selected chart cannot resolve certificate profiles: chart metadata is missing")
	}
	if err := valuesutil.ValidateAgainstSchema(selected, effective); err != nil {
		return nil, fmt.Errorf("invalid certificate target values: %w", err)
	}
	c := certificateHelperChart(selected)
	const name = "templates/certificate-target.json"
	c.Templates = append(c.Templates, &common.File{Name: name, Data: []byte(`{
"provider": {{ include "radius.authz.tls.provider" . | toJson }},
"trustDomain": {{ include "radius.authz.spiffe.trustDomain" . | toJson }},
"receivers": {{ include "radius.authz.certificates.receivers" . }},
"aggregation": {{ include "radius.authz.certificates.issuer" (dict "root" . "plane" "aggregation") }},
{{ if eq (include "radius.authz.tls.provider" .) "radius" }}"internal": {{ include "radius.authz.certificates.issuer" (dict "root" . "plane" "internal") }},
"internalSource": {{ include "radius.authz.certificates.publicSource" (dict "root" . "plane" "internal") }},{{ end }}
"aggregationSource": {{ include "radius.authz.certificates.publicSource" (dict "root" . "plane" "aggregation") }}
}`)})
	values, err := valuesutil.CoalesceValues(c, effective)
	if err != nil {
		return nil, fmt.Errorf("coalesce certificate target: %w", err)
	}
	rendered, err := engine.Render(c, common.Values{
		"Values": values, "Capabilities": common.DefaultCapabilities,
		"Release": map[string]any{"Name": "radius", "Namespace": namespace, "Service": "Helm"},
	})
	if err != nil {
		return nil, fmt.Errorf("selected chart cannot resolve certificate profiles: %w", err)
	}
	var result struct {
		Provider, TrustDomain             string
		Receivers                         []string
		Internal, Aggregation             certificateIssuerReference
		InternalSource, AggregationSource certificatePublicSource
	}
	if err := json.Unmarshal([]byte(rendered[c.Name()+"/"+name]), &result); err != nil {
		return nil, fmt.Errorf("decode selected chart certificate profiles: %w", err)
	}
	if result.Provider != "radius" && result.Provider != "external" {
		return nil, fmt.Errorf("selected chart returned an unsupported TLS provider")
	}
	target.Provider, target.TrustDomain, target.Receivers = result.Provider, result.TrustDomain, result.Receivers
	target.Planes["aggregation"] = certificatePlaneTarget{Issuer: result.Aggregation, Source: result.AggregationSource, Unresolved: true}
	if result.Provider == "radius" {
		target.Planes["internal"] = certificatePlaneTarget{Issuer: result.Internal, Source: result.InternalSource, Unresolved: true}
	}
	for _, plane := range target.Planes {
		if plane.Issuer.Group != "cert-manager.io" {
			return nil, fmt.Errorf("selected chart returned an unsupported certificate issuer group")
		}
		if _, err := parseCertificateIssuer(plane.Issuer.Kind + "/" + plane.Issuer.Name); err != nil {
			return nil, err
		}
	}
	return target, nil
}

func certificateHelperChart(selected *chart.Chart) *chart.Chart {
	c := cloneChart(selected)
	c.Templates = nil
	for _, file := range selected.Templates {
		if strings.HasPrefix(file.Name, "templates/_") {
			c.Templates = append(c.Templates, file)
		}
	}
	return c
}

// validateCAProfiles probes X.509 constraints with ephemeral certificates. It never
// signs with the operator key, and is not evidence of external backend issuance.
func validateCAProfiles(chain []*x509.Certificate, target *certificateTarget, namespace string, now time.Time) error {
	profiles := []x509.Certificate{{
		DNSNames:    []string{"ucp." + namespace + ".svc"},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}}
	for _, receiver := range target.Receivers {
		identity, err := url.Parse(mtls.SPIFFEID(target.TrustDomain, namespace, receiver))
		if err != nil {
			return fmt.Errorf("invalid selected SPIFFE identity: %w", err)
		}
		profiles = append(profiles, x509.Certificate{
			URIs:        []*url.URL{identity},
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			KeyUsage:    x509.KeyUsageDigitalSignature,
		})
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("prepare certificate constraint probe: %w", err)
	}
	for index, ca := range chain {
		probe := &x509.Certificate{
			SerialNumber: big.NewInt(1), Subject: ca.Subject,
			NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
			IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
		}
		// Copy the actual constraints and CA EKUs, including critical flags, into the
		// ephemeral issuer. Verify applies the standard library's DNS/URI semantics.
		for _, extension := range ca.Extensions {
			if extension.Id.String() == "2.5.29.30" || extension.Id.String() == "2.5.29.37" {
				probe.ExtraExtensions = append(probe.ExtraExtensions, extension)
			}
		}
		raw, err := x509.CreateCertificate(rand.Reader, probe, probe, &key.PublicKey, key)
		if err != nil {
			return fmt.Errorf("prepare CA constraint probe: %w", err)
		}
		probeCA, err := x509.ParseCertificate(raw)
		if err != nil {
			return fmt.Errorf("parse CA constraint probe: %w", err)
		}
		roots := x509.NewCertPool()
		roots.AddCert(probeCA)
		for _, profile := range profiles {
			profile.SerialNumber = big.NewInt(2)
			profile.NotBefore, profile.NotAfter = probe.NotBefore, probe.NotAfter
			raw, err := x509.CreateCertificate(rand.Reader, &profile, probeCA, &key.PublicKey, key)
			if err != nil {
				return fmt.Errorf("prepare leaf constraint probe: %w", err)
			}
			leaf, err := x509.ParseCertificate(raw)
			if err != nil {
				return fmt.Errorf("parse leaf constraint probe: %w", err)
			}
			for _, usage := range profile.ExtKeyUsage {
				if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{usage}}); err != nil {
					return fmt.Errorf("CA certificate %d cannot support the selected URI/DNS certificate profiles: %w", index+1, err)
				}
			}
		}
	}
	return nil
}
