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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
)

type rotationExternalIssuer struct {
	Group         string                       `json:"group"`
	Kind          string                       `json:"kind"`
	Name          string                       `json:"name"`
	UID           types.UID                    `json:"uid"`
	SpecSHA256    string                       `json:"specSHA256"`
	RequestPolicy map[string]map[string]string `json:"requestPolicy"`
}

func (i rotationExternalIssuer) validate() error {
	digest, err := hex.DecodeString(i.SpecSHA256)
	if err != nil || len(digest) != sha256.Size || i.SpecSHA256 != strings.ToLower(i.SpecSHA256) {
		return errors.New("rotation requires the pinned external issuer specification digest")
	}
	if i.Group != "cert-manager.io" || !slices.Contains([]string{"Issuer", "ClusterIssuer"}, i.Kind) ||
		i.UID == "" || i.Name == "" || len(validation.IsDNS1123Subdomain(i.Name)) != 0 || len(i.RequestPolicy) != 3 {
		return errors.New("rotation requires an explicit existing cert-manager issuer and public policy references")
	}
	for role, fields := range map[string][]string{
		"certManager": {"namespace", "deployment"}, "approverPolicy": {"namespace", "deployment"},
		"webhook": {"namespace", "service", "configuration"},
	} {
		ref := i.RequestPolicy[role]
		if len(ref) != len(fields) {
			return fmt.Errorf("rotation requires exact public %s prerequisite references", role)
		}
		for _, field := range fields {
			value := ref[field]
			if value == "" || len(validation.IsDNS1123Subdomain(value)) != 0 ||
				field == "namespace" && len(validation.IsDNS1123Label(value)) != 0 {
				return fmt.Errorf("rotation has invalid %s.%s prerequisite reference", role, field)
			}
		}
	}
	return nil
}

func (p rotationPlan) issuerReference() map[string]any {
	if p.ExternalIssuer != nil {
		return map[string]any{"group": p.ExternalIssuer.Group, "kind": p.ExternalIssuer.Kind, "name": p.ExternalIssuer.Name}
	}
	return map[string]any{"group": "cert-manager.io", "kind": "Issuer", "name": p.Issuer}
}

func (w rotationWorkload) issuerReference() map[string]any {
	group, kind := w.OldIssuerGroup, w.OldIssuerKind
	if group == "" {
		group = "cert-manager.io"
	}
	if kind == "" {
		kind = "Issuer"
	}
	return map[string]any{"group": group, "kind": kind, "name": w.OldIssuer}
}

func rotationIssuerRefsEqual(left, right map[string]any) bool {
	a, err := issuerReference(map[string]any{"issuerRef": left})
	if err != nil {
		return false
	}
	b, err := issuerReference(map[string]any{"issuerRef": right})
	return err == nil && a == b
}

func (i *Impl) checkRotationIssuerPolicy(ctx context.Context, options ClusterOptions, selected *chart.Chart,
	source *helm.Configuration, current map[string]any, kubeContext string, state rotationState) error {
	if state.Plan == nil || state.Plan.ExternalIssuer == nil {
		return errors.New("rotation has no pinned external issuer")
	}
	if _, err := validatePublicCA(state.NewCA, time.Now()); err != nil {
		return err
	}
	values := copyValues(current)
	policy := map[string]any{}
	for name, reference := range state.Plan.ExternalIssuer.RequestPolicy {
		fields := map[string]any{}
		for key, value := range reference {
			fields[key] = value
		}
		policy[name] = fields
	}
	setCertificateValues(values, map[string]any{"mode": "issuerRef", "issuerRef": state.Plan.issuerReference(),
		"caBundle": string(state.NewCA), "requestPolicy": policy})
	provider := "radius"
	if state.Plan.effectiveScope() == rotationAggregationOnly {
		provider = "external"
	}
	if err := unstructured.SetNestedField(values, provider, "global", "rbac", "mtls", "provider"); err != nil {
		return err
	}
	return i.renderIssuerPolicy(ctx, options, selected, source, values, kubeContext, true, &state)
}

func (i *Impl) rotationIssuerTarget(ctx context.Context, store *rotationStore, options ClusterOptions, effective map[string]any, kubeContext string) ([]byte, *rotationExternalIssuer, error) {
	values := copyValues(effective)
	selection := options
	selection.Certificates.RotationPhase = ""
	selection.RadiusReleaseValues = nil
	if options.Certificates.Specified() {
		policy := certificateValues(values)["requestPolicy"]
		setCertificateValues(values, map[string]any{"requestPolicy": policy})
	}
	if _, err := i.prepareCertificates(ctx, selection, nil, values, kubeContext, false); err != nil {
		return nil, nil, err
	}
	settings := certificateValues(values)
	reference, err := issuerReference(settings)
	if err != nil || certificateMode(values) != "issuerRef" {
		return nil, nil, errors.New("rotation target requires an explicit public issuer reference")
	}
	bundle, ok := settings["caBundle"].(string)
	if !ok {
		return nil, nil, errors.New("rotation target is missing its public CA")
	}
	if _, err := validatePublicCA([]byte(bundle), store.now()); err != nil {
		return nil, nil, err
	}
	pinned := &rotationExternalIssuer{Group: reference.Group, Kind: reference.Kind, Name: reference.Name}
	policy, err := json.Marshal(settings["requestPolicy"])
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(policy, &pinned.RequestPolicy); err != nil {
		return nil, nil, fmt.Errorf("invalid rotation policy references: %w", err)
	}
	resource, namespace := rotationIssuerResource, store.namespace
	if reference.Kind == "ClusterIssuer" {
		resource.Resource, namespace = "clusterissuers", ""
	}
	issuer, err := store.dynamic.Resource(resource).Namespace(namespace).Get(ctx, reference.Name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}
	if issuer.GetUID() == "" || issuer.GetResourceVersion() == "" || issuer.GetDeletionTimestamp() != nil {
		return nil, nil, errors.New("rotation issuer has no stable live identity")
	}
	pinned.UID = issuer.GetUID()
	pinned.SpecSHA256, err = rotationIssuerSpecDigest(issuer)
	if err != nil {
		return nil, nil, err
	}
	if err := pinned.validate(); err != nil {
		return nil, nil, err
	}
	return []byte(bundle), pinned, nil
}

func (s *rotationStore) externalRotationIssuer(ctx context.Context, plan *rotationPlan) (*unstructured.Unstructured, error) {
	if plan.ExternalIssuer == nil {
		return nil, errors.New("rotation has no external issuer identity")
	}
	if err := plan.ExternalIssuer.validate(); err != nil {
		return nil, err
	}
	resource := rotationIssuerResource
	namespace := s.namespace
	if plan.ExternalIssuer.Kind == "ClusterIssuer" {
		resource.Resource, namespace = "clusterissuers", ""
	}
	issuer, err := s.dynamic.Resource(resource).Namespace(namespace).Get(ctx, plan.ExternalIssuer.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if issuer.GetUID() != plan.ExternalIssuer.UID || issuer.GetResourceVersion() == "" || issuer.GetDeletionTimestamp() != nil {
		return nil, errors.New("the pinned external issuer was replaced or is terminating")
	}
	digest, err := rotationIssuerSpecDigest(issuer)
	if err != nil {
		return nil, err
	}
	if digest != plan.ExternalIssuer.SpecSHA256 {
		return nil, errors.New("the pinned external issuer specification changed")
	}
	return issuer, nil
}

func rotationIssuerSpecDigest(issuer *unstructured.Unstructured) (string, error) {
	spec, found, err := unstructured.NestedMap(issuer.Object, "spec")
	if err != nil || !found || len(spec) == 0 {
		return "", errors.New("external issuer has no valid specification")
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
