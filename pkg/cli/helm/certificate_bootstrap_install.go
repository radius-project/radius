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

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz"
	helm "helm.sh/helm/v4/pkg/action"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (i *Impl) prepareAggregationBootstrap(ctx context.Context, options ClusterOptions, selected *chart.Chart,
	configuration *helm.Configuration, values map[string]any, kubeContext string, imported *certificatePlan, target *certificateTarget) (*initialCertificateInstaller, error) {
	store, err := i.rotationStoreForContext(kubeContext, options.Radius.Namespace)
	if err != nil {
		return nil, err
	}
	current, found, readErr := store.bootstrapIntent(ctx)
	if readErr != nil {
		return nil, readErr
	}
	effective := valuesutil.CoalesceTables(copyValues(values), copyValues(options.RadiusReleaseValues))
	effective = valuesutil.CoalesceTables(effective, copyValues(selected.Values))
	mode, err := resolveAuthzMode(nil, effective)
	if err != nil {
		return nil, err
	}
	pending := found && current.Phase != bootstrapComplete
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
	if mode == authz.ModeOff {
		if pending || namespaceIntent != nil || initialErr == nil && initial.Phase != "complete" {
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
	if target == nil || selected.Metadata == nil || selected.Metadata.Annotations["radius.io/initial-certificate-protocol"] != "1" {
		return nil, errors.New("fresh installation requires the selected chart's initial certificate protocol and validated target")
	}
	chartHash, err := bootstrapDigest(selected)
	if err != nil {
		return nil, err
	}
	intentHash, err := initialCertificateValuesDigest(effective)
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
