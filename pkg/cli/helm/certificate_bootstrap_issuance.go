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
	"io"
	"strings"

	"github.com/radius-project/radius/pkg/authz/mtls"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/release"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

func (i *Impl) renderBootstrapRoots(ctx context.Context, selected *chart.Chart, options ClusterOptions, values map[string]any, kubeContext string) ([]*unstructured.Unstructured, error) {
	if certificateMode(values) != "selfSigned" {
		return nil, errors.New("root preparation is limited to the selected selfSigned profile")
	}
	renderedChart := cloneChart(selected)
	templates := []*common.File{}
	found := false
	for _, file := range selected.Templates {
		if strings.HasPrefix(file.Name, "templates/_") || file.Name == "templates/global/certificates.yaml" {
			templates = append(templates, file)
			found = found || file.Name == "templates/global/certificates.yaml"
		}
	}
	if !found {
		return nil, errors.New("selected chart does not define the selfSigned bootstrap resources")
	}
	renderedChart.Templates = templates
	manifest, err := i.renderBootstrapChart(ctx, renderedChart, options, values, kubeContext)
	if err != nil {
		return nil, err
	}
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	var issuer, certificate *unstructured.Unstructured
	for {
		object := &unstructured.Unstructured{}
		if err := decoder.Decode(object); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		switch {
		case object.GetKind() == "Issuer" && object.GetName() == "radius-selfsigned-bootstrap":
			if issuer != nil {
				return nil, errors.New("selected chart repeats the bootstrap issuer")
			}
			issuer = object
		case object.GetKind() == "Certificate" && object.GetName() == "radius-ca":
			if certificate != nil {
				return nil, errors.New("selected chart repeats the bootstrap CA Certificate")
			}
			certificate = object
		}
	}
	if issuer == nil || certificate == nil ||
		issuer.GetAPIVersion() != "cert-manager.io/v1" || certificate.GetAPIVersion() != "cert-manager.io/v1" ||
		issuer.GetNamespace() != options.Radius.Namespace || certificate.GetNamespace() != options.Radius.Namespace {
		return nil, errors.New("selected chart has no exact namespace-local bootstrap issuer and CA Certificate")
	}
	return []*unstructured.Unstructured{issuer, certificate}, nil
}

func (i *Impl) renderBootstrapChart(ctx context.Context, selected *chart.Chart, options ClusterOptions, values map[string]any, kubeContext string) (string, error) {
	configuration, err := i.cluster().RenderConfiguration(kubeContext, options.Radius.Namespace)
	if err != nil {
		return "", err
	}
	install := newInstallClient(configuration, options.Radius.ReleaseName, options.Radius.Namespace, false, options.Radius.Timeout)
	install.DryRunStrategy = helm.DryRunServer
	rendered, err := install.RunWithContext(ctx, cloneChart(selected), copyValues(values))
	if err != nil {
		return "", err
	}
	accessor, err := release.NewAccessor(rendered)
	if err != nil {
		return "", err
	}
	return accessor.Manifest(), nil
}

// resourcePlan contains desired issuance names and public pins, not invented
// APIService or workload identities. Binding those identities is a later step.
func (s aggregationBootstrapState) resourcePlan() (*rotationPlan, error) {
	digest, roots, err := mtls.TrustBundleDigest(s.CA, s.CAPinnedAt)
	if err != nil || len(roots) != 1 {
		return nil, errors.New("bootstrap issuance requires its pinned dedicated CA")
	}
	suffix := strings.ReplaceAll(s.ID, "-", "")[:16]
	issuer := "radius-ca-" + suffix
	if s.External != nil {
		issuer = s.External.Name
	}
	return &rotationPlan{Scope: s.Scope, TargetMode: s.Mode, TargetTrustSHA256: digest,
		TargetTrustMap: "radius-mtls-ca-" + digest[:32], SigningSecret: s.Signer, SigningUID: s.SignerUID,
		ExternalIssuer: s.External, Issuer: issuer, ServingSecret: "ucp-cert-aggregation-" + suffix}, nil
}

func (s *rotationStore) prepareBootstrapCertificates(ctx context.Context, state aggregationBootstrapState) error {
	plan, err := state.resourcePlan()
	if err != nil {
		return err
	}
	if err := s.checkBootstrapSigner(ctx, state); err != nil {
		return err
	}
	labels := map[string]any{rotationOwner: s.release, rotationTransaction: state.ID}
	if state.External == nil {
		if state.SignerUID == "" {
			return errors.New("bootstrap issuance has no observed signing Secret identity")
		}
		issuer := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "cert-manager.io/v1", "kind": "Issuer",
			"metadata": map[string]any{"name": plan.Issuer, "namespace": s.namespace, "labels": labels},
			"spec":     map[string]any{"ca": map[string]any{"secretName": plan.SigningSecret}},
		}}
		if err := s.ensureBootstrapResource(ctx, state.ID, rotationIssuerResource, issuer, false); err != nil {
			return err
		}
	}
	certificate := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
		"metadata": map[string]any{"name": plan.ServingSecret, "namespace": s.namespace, "labels": labels},
		"spec":     rotationAggregationCertificateSpec(rotationState{Plan: plan}, s.namespace),
	}}
	if err := s.checkBootstrapSigner(ctx, state); err != nil {
		return err
	}
	if err := s.ensureBootstrapResource(ctx, state.ID, rotationCertificateResource, certificate, false); err != nil {
		return err
	}
	return nil
}

func (s *rotationStore) checkBootstrapSigner(ctx context.Context, state aggregationBootstrapState) error {
	if state.Mode == "selfSigned" {
		root, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, "radius-ca", metav1.GetOptions{})
		if err != nil {
			return err
		}
		if root.UID != state.RootSecretUID || root.DeletionTimestamp != nil {
			return errors.New("bootstrap root Secret identity changed")
		}
		ca, err := validateSigningCA(root.Data[corev1.TLSCertKey], root.Data[corev1.TLSPrivateKeyKey], s.now())
		if err != nil || !sameCACertificate(string(state.CA), ca) {
			return errors.New("bootstrap root Secret changed its selected CA or private key")
		}
	}
	plan, err := state.resourcePlan()
	if err != nil {
		return err
	}
	if state.External == nil {
		return s.checkPlannedSigner(ctx, rotationState{Plan: plan, NewCA: state.CA})
	}
	if _, err := s.externalRotationIssuer(ctx, plan); err != nil {
		return err
	}
	if s.verifyBootstrapIssuerPolicy == nil {
		return errors.New("bootstrap issuer requires current live chart policy verification")
	}
	if err := s.verifyBootstrapIssuerPolicy(ctx, state); err != nil {
		return err
	}
	_, err = s.externalRotationIssuer(ctx, plan)
	return err
}

func (s *rotationStore) checkBootstrapLeaf(ctx context.Context, state aggregationBootstrapState) error {
	plan, err := state.resourcePlan()
	if err != nil {
		return err
	}
	secret, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, plan.ServingSecret, metav1.GetOptions{})
	if err != nil {
		return err
	}
	_, err = rotationLeafDigest(secret, state.CA, s.namespace, "ucp", false, s.now())
	return err
}
