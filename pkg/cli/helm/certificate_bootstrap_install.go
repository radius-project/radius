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

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz"
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// failedBootstrapRetry is an exception for a published first-install transaction,
// not a provider baseline for maintenance of arbitrary failed releases.
func (i *Impl) failedBootstrapRetry(ctx context.Context, options ClusterOptions, selected *chart.Chart, target *certificateTarget, previous *releasev1.Release, kubeContext string) (func() error, error) {
	store, err := i.rotationStoreForContext(kubeContext, options.Radius.Namespace)
	if err != nil {
		return nil, err
	}
	journal, state, err := readInitialCertificateJournal(ctx, store)
	if err != nil {
		return nil, fmt.Errorf("cannot establish deployed TLS provider: no deployed Radius revision; identical bootstrap retry requires its published journal: %w", err)
	}
	if state.Phase != "published" || previous.Labels[initialCertificateIntent] != state.ID ||
		target.mode == authz.ModeOff || target.Mode != state.Mode || target.Provider != state.Provider {
		return nil, errors.New("failed bootstrap retry does not match the published transaction")
	}
	for _, c := range []*chart.Chart{previous.Chart, selected} {
		digest, err := bootstrapDigest(c)
		if err != nil || digest != state.ChartSHA256 {
			return nil, errors.New("failed bootstrap retry changed the recorded chart")
		}
	}
	for _, values := range []map[string]any{resolveEffectiveChartValues(previous.Chart.Values, nil, previous.Config), target.Values} {
		digest, err := bootstrapDigest(values)
		if err != nil || digest != state.ValuesSHA256 || digest != state.IntentSHA256 {
			return nil, errors.New("failed bootstrap retry changed the recorded values")
		}
	}
	recheck := func() error {
		current, _, err := readInitialCertificateJournal(ctx, store)
		if err != nil {
			return err
		}
		if current.UID != journal.UID || current.ResourceVersion != journal.ResourceVersion {
			return errors.New("failed bootstrap journal changed since preflight")
		}
		namespace, err := store.kube.CoreV1().Namespaces().Get(ctx, store.namespace, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if namespace.UID != state.NamespaceUID || namespace.DeletionTimestamp != nil {
			return errors.New("failed bootstrap namespace identity changed")
		}
		return nil
	}
	return recheck, recheck()
}

func (i *Impl) prepareAggregationBootstrap(ctx context.Context, options ClusterOptions, selected *chart.Chart,
	configuration *helm.Configuration, values map[string]any, kubeContext string, imported *certificatePlan, target *certificateTarget) (*initialCertificateInstaller, error) {
	store, err := i.rotationStoreForContext(kubeContext, options.Radius.Namespace)
	if err != nil {
		return nil, err
	}
	found, readErr := store.hasLegacyBootstrap(ctx)
	if readErr != nil {
		return nil, readErr
	}
	if target == nil {
		return nil, errors.New("bootstrap requires a validated certificate target")
	}
	journal, initial, initialErr := readInitialCertificateJournal(ctx, store)
	if initialErr != nil && !apierrors.IsNotFound(initialErr) {
		return nil, initialErr
	}
	var namespaceIntent *initialCertificateState
	if apierrors.IsNotFound(initialErr) {
		namespace, err := store.kube.CoreV1().Namespaces().Get(ctx, store.namespace, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
		if err == nil && namespace.Annotations[initialCertificateIntent] != "" {
			intent, err := decodeInitialCertificateState(namespace.Annotations[initialCertificateIntent], store, false)
			if err != nil {
				return nil, err
			}
			namespaceIntent = &intent
		}
	}
	if target.mode == authz.ModeOff {
		if found || namespaceIntent != nil || initialErr == nil && initial.Phase != "complete" {
			return nil, errors.New("unfinished bootstrap cannot be bypassed by disabling authorization; resume the identical install")
		}
		return nil, nil
	}
	if found {
		return nil, errors.New("legacy bootstrap requires explicit pre-token recovery")
	}
	if initialErr == nil && initial.Phase == "complete" {
		if !options.RadiusInstalled && options.RadiusReleaseValues == nil {
			return nil, errors.New("completed certificate bootstrap belongs to a previous installation; do not adopt it")
		}
		return nil, nil
	}
	if apierrors.IsNotFound(initialErr) && (options.RadiusInstalled || options.RadiusReleaseValues != nil) {
		if namespaceIntent != nil {
			return nil, errors.New("initial certificate journal is missing; restore it before changing this installation")
		}
		return nil, nil
	}
	if selected.Metadata == nil || selected.Metadata.Annotations["radius.io/initial-certificate-protocol"] != "1" {
		return nil, errors.New("fresh installation requires the selected chart's initial certificate protocol and validated target")
	}
	chartHash, err := bootstrapDigest(selected)
	if err != nil {
		return nil, err
	}
	intentHash, err := initialCertificateValuesDigest(target.Values)
	if err != nil {
		return nil, err
	}
	if initialErr == nil && (initial.IntentSHA256 != intentHash || initial.ChartSHA256 != chartHash ||
		initial.Mode != target.Mode || initial.Provider != target.Provider) {
		return nil, errors.New("bootstrap retry differs from the recorded chart or certificate intent")
	}
	if namespaceIntent != nil && (namespaceIntent.IntentSHA256 != intentHash || namespaceIntent.ChartSHA256 != chartHash ||
		namespaceIntent.Mode != target.Mode || namespaceIntent.Provider != target.Provider) {
		return nil, errors.New("namespace retains a different initial certificate intent")
	}
	if apierrors.IsNotFound(initialErr) {
		initial = initialCertificateState{Version: 1, ID: uuid.NewString(), Namespace: store.namespace, Release: store.release,
			Mode: target.Mode, Provider: target.Provider, Phase: "preparing", ChartSHA256: chartHash, IntentSHA256: intentHash,
			Roots: map[string]aggregationBootstrapPin{}, Anchors: map[string]initialCertificateAnchor{},
			Sources: map[string]certificateTrustProjection{}, Issuers: map[string]certificateIssuerSnapshot{}}
	}
	return &initialCertificateInstaller{impl: i, store: store, options: options, selected: selected, helm: configuration,
		values: values, kubeContext: kubeContext, importPlan: imported, target: target, state: initial, journal: journal}, nil
}
