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
	"k8s.io/apimachinery/pkg/types"
)

// certificateTarget contains public desired state only. It does not authorize writes,
// approve a transition, or prove that an external issuer can actually issue a leaf.
type certificateTarget struct {
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

func (i *Impl) prepareSelectedCertificates(ctx context.Context, options ClusterOptions, selected *chart.Chart, current map[string]any, kubeContext string, installed bool) (*certificateTarget, *certificatePlan, error) {
	target, plan, err := i.prepareCertificateTarget(ctx, options, selected, current, kubeContext)
	if err != nil {
		return nil, nil, err
	}
	if installed {
		if err := unchangedCertificateSelection(options.RadiusReleaseValues, current); err != nil {
			return nil, nil, fmt.Errorf("%w: %w", errCertificateObserverUnavailable, err)
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
	plan, err := i.prepareCertificateSelection(ctx, options, selected.Values, current, kubeContext)
	if err != nil {
		return nil, nil, err
	}
	if plan != nil && plan.secret != nil {
		plan.requiresLease = options.RadiusInstalled || options.RadiusReleaseValues != nil
	}
	effective := valuesutil.CoalesceTables(copyValues(current), copyValues(options.RadiusReleaseValues))
	effective = valuesutil.CoalesceTables(effective, copyValues(selected.Values))
	target, err := certificateProfilePlan(selected, effective, options.Radius.Namespace)
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
func certificateProfilePlan(selected *chart.Chart, effective map[string]any, namespace string) (*certificateTarget, error) {
	mode, err := resolveAuthzMode(nil, effective)
	if err != nil {
		return nil, err
	}
	target := &certificateTarget{
		Mode: certificateMode(effective), Values: copyValues(effective),
		Planes: map[string]certificatePlaneTarget{},
	}
	if mode == authz.ModeOff {
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
"internal": {{ include "radius.authz.certificates.issuer" (dict "root" . "plane" "internal") }},
"aggregationSource": {{ include "radius.authz.certificates.publicSource" (dict "root" . "plane" "aggregation") }},
"internalSource": {{ include "radius.authz.certificates.publicSource" (dict "root" . "plane" "internal") }}
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

// certificatePolicyChart retains the selected chart's real policy evaluator without
// rendering lifecycle resources. Only the lifecycle may authorize a target transition.
func certificatePolicyChart(selected *chart.Chart) *chart.Chart {
	c := certificateHelperChart(selected)
	c.Templates = append(c.Templates, &common.File{
		Name: "templates/certificate-target-policy.yaml",
		Data: []byte(`{{ include "radius.authz.issuerPolicy.validate" . }}`),
	})
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
			URIs: []*url.URL{identity},
			// CSI v0.15.1 requires these usages, although callers use tokens, not client certificates.
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
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
