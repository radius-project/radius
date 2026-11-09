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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	helm "helm.sh/helm/v4/pkg/action"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
)

const (
	bootstrapIDValue         = "aggregationBootstrap"
	bootstrapInvocationValue = "aggregationBootstrapInvocation"
	bootstrapNamespaceIntent = "radius.io/aggregation-bootstrap-intent"
)

type aggregationBootstrapInstaller struct {
	impl         *Impl
	store        *rotationStore
	options      ClusterOptions
	chart        *chart.Chart
	helm         *helm.Configuration
	values       map[string]any
	kubeContext  string
	importPlan   *certificatePlan
	runner       *rotationRunner
	dependencies func(context.Context) error
}

func bootstrapValuesDigest(values map[string]any) (string, error) {
	copy := copyValues(values)
	unstructured.RemoveNestedField(copy, "global", "rbac", bootstrapInvocationValue)
	return bootstrapDigest(copy)
}

func (b *aggregationBootstrapInstaller) requested(ctx context.Context) (aggregationBootstrapState, error) {
	now := b.store.now().UTC()
	state := aggregationBootstrapState{Version: 1, ID: uuid.NewString(), Namespace: b.store.namespace, Release: b.store.release,
		CreatedAt: now, Phase: bootstrapPreparing, Mode: certificateMode(b.values),
		Scope: rotationComponentsAndAggregation, Resources: map[string]aggregationBootstrapPin{}}
	provider, _, err := unstructured.NestedString(b.values, "global", "rbac", "mtls", "provider")
	if err == nil && provider == "" {
		provider = "radius"
	}
	if err != nil || provider != "radius" && provider != "external" {
		return state, errors.New("bootstrap requires an explicit managed or external provider")
	}
	if provider == "external" {
		state.Scope = rotationAggregationOnly
	} else {
		enabled, _, err := unstructured.NestedBool(b.values, "de", "ucpMTLS", "enabled")
		if err != nil || !enabled {
			return state, errors.New("managed bootstrap requires the deployment-engine mTLS opt-in and compatible process-evidence images")
		}
	}
	state.ProofPort, err = bootstrapProofPort(b.values)
	if err != nil {
		return state, err
	}
	state.ChartSHA256, err = bootstrapDigest(b.chart)
	if err != nil {
		return state, err
	}
	valuesHash, err := bootstrapValuesDigest(b.values)
	if err != nil {
		return state, err
	}
	state.IntentSHA256, err = bootstrapDigest([]string{state.ChartSHA256, valuesHash})
	if err != nil {
		return state, err
	}
	switch state.Mode {
	case "selfSigned":
	case "caSecret":
		settings := certificateValues(b.values)
		name, nameOK := settings["caSecret"].(string)
		bundle, bundleOK := settings["caBundle"].(string)
		if !nameOK || !bundleOK {
			return state, errors.New("bootstrap requires the validated signing Secret and public CA")
		}
		state.Signer, state.CA, state.CAPinnedAt = name, []byte(bundle), now
		secret, err := b.store.kube.CoreV1().Secrets(state.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			state.SignerUID = secret.UID
		} else if !apierrors.IsNotFound(err) || b.importPlan == nil || b.importPlan.secret == nil || b.importPlan.secret.Name != name {
			return state, err
		}
	case "issuerRef":
		selection := b.options
		selection.Certificates = CertificateOptions{}
		state.CA, state.External, err = b.impl.rotationIssuerTarget(ctx, b.store, selection, b.values, b.kubeContext)
		if err != nil {
			return state, err
		}
		state.CAPinnedAt = now
	default:
		return state, errors.New("unsupported aggregation bootstrap certificate mode")
	}
	_, current, err := b.store.readBootstrap(ctx)
	if err == nil && current.ChartSHA256 == state.ChartSHA256 && current.InstallValuesSHA256 == valuesHash {
		// A failed Helm install may have persisted the generated proof settings.
		// Only the exact recorded install values can resume that same intent.
		state.IntentSHA256 = current.IntentSHA256
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return state, err
	}
	return state, state.validate(b.store.now())
}

func (b *aggregationBootstrapInstaller) ensureNamespace(ctx context.Context, requested aggregationBootstrapState) (aggregationBootstrapState, error) {
	namespace, err := b.store.kube.CoreV1().Namespaces().Get(ctx, b.store.namespace, metav1.GetOptions{})
	if err == nil {
		if namespace.DeletionTimestamp != nil {
			return requested, errors.New("bootstrap namespace is terminating")
		}
		if _, _, err := b.store.readBootstrap(ctx); apierrors.IsNotFound(err) {
			if raw := namespace.Annotations[bootstrapNamespaceIntent]; raw != "" {
				original, err := decodeAggregationBootstrap(raw)
				if err != nil {
					return requested, err
				}
				if err := original.validate(b.store.now()); err != nil {
					return requested, err
				}
				if original.IntentSHA256 != requested.IntentSHA256 || original.Namespace != requested.Namespace || original.Release != requested.Release {
					return requested, errors.New("namespace retains a different bootstrap intent; investigate before retrying")
				}
				return original, nil
			}
		} else if err != nil {
			return requested, err
		}
		return requested, nil
	}
	if !apierrors.IsNotFound(err) {
		return requested, err
	}
	data, err := json.Marshal(requested)
	if err != nil || len(data) > maxRotationBytes {
		return requested, errors.New("namespace bootstrap intent exceeds its size limit")
	}
	// No namespace-scoped Lease or journal can exist yet. The namespace create
	// itself records public intent; all subsequent owned writes hold the Lease.
	_, err = b.store.kube.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: b.store.namespace, Annotations: map[string]string{bootstrapNamespaceIntent: string(data)}},
	}, metav1.CreateOptions{FieldManager: FieldManager})
	return requested, err
}

func (b *aggregationBootstrapInstaller) run(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, effectiveTimeout(b.options.Radius.Timeout))
	defer cancel()
	requested, err := b.requested(ctx)
	if err != nil {
		return err
	}
	requested, err = b.ensureNamespace(ctx, requested)
	if err != nil {
		return err
	}
	return withIdentityOperation(ctx, b.store.kube, b.store.namespace, func(operation *identityOperation) error {
		b.store.guardWrite = func(ctx context.Context) error {
			if err := operation.checkProviderJournal(ctx, b.store.release, b.impl.validateCompletedProviderJournal); err != nil {
				return err
			}
			_, state, err := b.store.readBootstrap(ctx)
			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if state.External != nil || state.SignerUID != "" {
				if err := b.store.checkBootstrapSigner(ctx, state); err != nil {
					return err
				}
			}
			if err := b.store.checkBootstrapResources(ctx, state); err != nil {
				return err
			}
			return operation.check(ctx)
		}
		b.store.verifyBootstrapIssuerPolicy = func(ctx context.Context, state aggregationBootstrapState) error {
			plan, err := state.resourcePlan()
			if err != nil {
				return err
			}
			values := copyValues(b.values)
			settings := certificateValues(values)
			settings["issuerRef"], settings["caBundle"] = plan.issuerReference(), string(state.CA)
			setCertificateValues(values, settings)
			return b.impl.checkIssuerPolicy(ctx, b.options, b.chart, b.helm, values, b.kubeContext, false)
		}
		if requested.External != nil {
			if err := b.store.checkBootstrapSigner(ctx, requested); err != nil {
				return err
			}
		}
		b.store.activeBootstrap = requested.ID
		_, state, err := b.store.beginBootstrap(ctx, requested)
		if err != nil {
			return err
		}
		b.store.activeBootstrap = state.ID
		if state.Phase == bootstrapPreparing && b.dependencies != nil {
			if err := b.store.checkWrite(ctx); err != nil {
				return err
			}
			if err := b.dependencies(ctx); err != nil {
				return err
			}
		}
		if state.Phase == bootstrapPreparing {
			if err := b.prepare(ctx, state); err != nil {
				return err
			}
		}
		journal, state, err := b.store.readBootstrap(ctx)
		if err != nil {
			return err
		}
		if state.Phase == bootstrapInstalling {
			if err := b.store.checkBootstrapSigner(ctx, state); err != nil {
				return err
			}
			if err := b.store.checkBootstrapLeaf(ctx, state); err != nil {
				return err
			}
			values, err := b.installValues(state)
			if err != nil {
				return err
			}
			digest, err := bootstrapValuesDigest(values)
			if err != nil || digest != state.InstallValuesSHA256 {
				return errors.New("bootstrap retry changed the pinned install settings")
			}
			if err := unstructured.SetNestedField(values, string(operation.lease.UID), "global", "rbac", bootstrapInvocationValue); err != nil {
				return err
			}
			manifest, err := b.impl.renderBootstrapChart(ctx, b.chart, b.options, values, b.kubeContext)
			if err != nil {
				return err
			}
			if err := b.store.checkBootstrapInstall(ctx, state.ID, manifest, false); err != nil {
				return err
			}
			if err := b.store.checkWrite(ctx); err != nil {
				return err
			}
			installOptions := b.options.Radius.ChartOptions
			installOptions.Reinstall = true
			if err := NewHelmAction(b.impl.Helm).ApplyHelmChart(b.kubeContext, b.chart, b.helm, installOptions, values); err != nil {
				pinErr := b.store.checkBootstrapInstall(ctx, state.ID, manifest, false)
				return fmt.Errorf("bootstrap install is unfinished; retry the identical install settings: %w", errors.Join(err, pinErr))
			}
			if err := b.store.checkBootstrapInstall(ctx, state.ID, manifest, true); err != nil {
				return err
			}
			journal, state, err = b.store.readBootstrap(ctx)
			if err != nil {
				return err
			}
			next := state
			next.Phase = bootstrapVerifying
			if _, err := b.store.updateBootstrap(ctx, journal, state, next); err != nil {
				return err
			}
		}
		journal, state, err = b.store.readBootstrap(ctx)
		if err != nil {
			return err
		}
		if state.Phase == bootstrapComplete {
			_, lifecycle, err := b.store.read(ctx)
			if err != nil {
				return err
			}
			if lifecycle.ID != state.ID || !lifecycle.Bootstrap {
				return errors.New("completed bootstrap has no matching initial lifecycle")
			}
			return b.runner.verifyBootstrapLifecycle(ctx, lifecycle)
		}
		lifecycle, err := b.store.bindBootstrapLifecycle(ctx, state)
		if err != nil {
			return err
		}
		if err := b.runner.wait(ctx, "fresh aggregation and component evidence", func() error {
			return b.runner.verifyBootstrapLifecycle(ctx, lifecycle)
		}); err != nil {
			return err
		}
		if err := b.store.checkBootstrapSigner(ctx, state); err != nil {
			return err
		}
		if err := b.store.recordBootstrapLifecycle(ctx, lifecycle); err != nil {
			return err
		}
		if err := b.store.reconcileTrust(ctx, state.ID); err != nil {
			return err
		}
		if err := b.runner.verifyBootstrapLifecycle(ctx, lifecycle); err != nil {
			return err
		}
		journal, state, err = b.store.readBootstrap(ctx)
		if err != nil {
			return err
		}
		next := state
		next.Phase = bootstrapComplete
		_, err = b.store.updateBootstrap(ctx, journal, state, next)
		return err
	})
}

func (b *aggregationBootstrapInstaller) prepare(ctx context.Context, state aggregationBootstrapState) error {
	if state.Mode == "selfSigned" {
		roots, err := b.impl.renderBootstrapRoots(ctx, b.chart, b.options, b.values, b.kubeContext)
		if err != nil {
			return err
		}
		for _, object := range roots {
			resource := rotationIssuerResource
			if object.GetKind() == "Certificate" {
				resource = rotationCertificateResource
			}
			if err := b.store.ensureBootstrapResource(ctx, state.ID, resource, object, true); err != nil {
				return err
			}
		}
		var target []byte
		var signer string
		if err := b.runner.wait(ctx, "selected chart root issuance", func() error {
			var err error
			b.importPlan, target, signer, _, err = b.impl.rotationTarget(ctx, b.options, b.values, b.kubeContext)
			return err
		}); err != nil {
			return err
		}
		journal, current, err := b.store.readBootstrap(ctx)
		if err != nil {
			return err
		}
		if len(current.CA) != 0 && (!bytes.Equal(current.CA, target) || current.Signer != signer) {
			return errors.New("selected chart root changed after bootstrap pinned its public CA")
		}
		if len(current.CA) == 0 {
			root, err := b.store.kube.CoreV1().Secrets(b.store.namespace).Get(ctx, "radius-ca", metav1.GetOptions{})
			if err != nil {
				return err
			}
			if root.UID == "" || root.DeletionTimestamp != nil {
				return errors.New("issued bootstrap root has no stable Secret identity")
			}
			ca, err := validateSigningCA(root.Data[corev1.TLSCertKey], root.Data[corev1.TLSPrivateKeyKey], b.store.now())
			if err != nil || !sameCACertificate(string(target), ca) {
				return errors.New("bootstrap root changed while preparing its retained signing snapshot")
			}
			next := current
			next.CA, next.Signer, next.CAPinnedAt = target, signer, b.store.now().UTC()
			next.RootSecretUID = root.UID
			if _, err := b.store.updateBootstrap(ctx, journal, current, next); err != nil {
				return err
			}
		}
	}
	if b.importPlan != nil {
		b.importPlan.beforeWrite = b.store.checkWrite
	}
	if err := b.impl.applyCertificatePlan(ctx, b.importPlan); err != nil {
		return err
	}
	journal, state, err := b.store.readBootstrap(ctx)
	if err != nil {
		return err
	}
	if state.External == nil && state.SignerUID == "" {
		secret, err := b.store.kube.CoreV1().Secrets(b.store.namespace).Get(ctx, state.Signer, metav1.GetOptions{})
		if err != nil {
			return err
		}
		next := state
		next.SignerUID = secret.UID
		if err := b.store.checkBootstrapSigner(ctx, next); err != nil {
			return err
		}
		if _, err := b.store.updateBootstrap(ctx, journal, state, next); err != nil {
			return err
		}
		state = next
	}
	if err := b.store.prepareBootstrapCertificates(ctx, state); err != nil {
		return err
	}
	if err := b.runner.wait(ctx, "selected CA aggregation leaf issuance", func() error {
		return b.store.checkBootstrapLeaf(ctx, state)
	}); err != nil {
		return err
	}
	if err := b.store.checkBootstrapSigner(ctx, state); err != nil {
		return err
	}
	if _, _, err := b.store.publishComponentTrust(ctx, state.CA); err != nil {
		return err
	}
	journal, state, err = b.store.readBootstrap(ctx)
	if err != nil {
		return err
	}
	next := state
	next.Phase = bootstrapInstalling
	next.TrustSHA256, _, err = mtls.TrustBundleDigest(state.CA, b.store.now())
	if err != nil {
		return err
	}
	values, err := b.installValues(next)
	if err != nil {
		return err
	}
	next.InstallValuesSHA256, err = bootstrapValuesDigest(values)
	if err != nil {
		return err
	}
	_, err = b.store.updateBootstrap(ctx, journal, state, next)
	return err
}

func bootstrapProofPort(values map[string]any) (uint16, error) {
	var prometheus struct {
		Enabled bool   `json:"enabled"`
		Port    uint16 `json:"port"`
	}
	settings, _, err := unstructured.NestedMap(values, "global", "prometheus")
	if err != nil {
		return 0, err
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return 0, err
	}
	if err := json.Unmarshal(data, &prometheus); err != nil || !prometheus.Enabled || prometheus.Port == 0 {
		return 0, errors.New("bootstrap requires an enabled fixed UCP management listener")
	}
	return prometheus.Port, nil
}

func (b *aggregationBootstrapInstaller) installValues(state aggregationBootstrapState) (map[string]any, error) {
	values := copyValues(b.values)
	unstructured.RemoveNestedField(values, "global", "rbac", bootstrapInvocationValue)
	if err := unstructured.SetNestedField(values, state.ID, "global", "rbac", bootstrapIDValue); err != nil {
		return nil, err
	}
	// This is desired Helm configuration, not completion evidence. The bootstrap
	// journal continues to block lifecycle changes until the observations pass.
	if err := unstructured.SetNestedField(values, state.ID, "global", "rbac", "completedCARotation"); err != nil {
		return nil, err
	}
	if state.Scope == rotationComponentsAndAggregation {
		proof := map[string]any{"enabled": true, "expectedTrustSHA256": state.TrustSHA256}
		if err := unstructured.SetNestedField(values, proof, "global", "rbac", "startupProof"); err != nil {
			return nil, err
		}
		if err := unstructured.SetNestedField(values, proof, "de", "ucpMTLS", "startupProof"); err != nil {
			return nil, err
		}
	}
	coalesced, err := valuesutil.CoalesceValues(b.chart, values)
	return map[string]any(coalesced), err
}

func (i *Impl) newBootstrapInstaller(store *rotationStore, config *rest.Config, options ClusterOptions, selected *chart.Chart,
	configuration *helm.Configuration, values map[string]any, kubeContext string, imported *certificatePlan) *aggregationBootstrapInstaller {
	return &aggregationBootstrapInstaller{impl: i, store: store, options: options, chart: selected, helm: configuration,
		values: values, kubeContext: kubeContext, importPlan: imported,
		runner: &rotationRunner{store: store, config: config, log: i.logInfo}}
}

func (i *Impl) prepareAggregationBootstrap(ctx context.Context, options ClusterOptions, selected *chart.Chart,
	configuration *helm.Configuration, values map[string]any, kubeContext string, imported *certificatePlan) (*aggregationBootstrapInstaller, error) {
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
	if mode == authz.ModeOff {
		if pending {
			return nil, errors.New("unfinished bootstrap cannot be bypassed by disabling authorization; resume the identical install")
		}
		return nil, nil
	}
	if !pending && (options.RadiusInstalled || options.RadiusReleaseValues != nil || found) {
		return nil, nil
	}
	if selected.Metadata == nil || selected.Metadata.Annotations["radius.io/aggregation-bootstrap-protocol"] != "1" {
		return nil, errors.New("fresh enabled installation requires a chart supporting configured-CA aggregation bootstrap")
	}
	effective, err = valuesutil.CoalesceValues(selected, effective)
	if err != nil {
		return nil, err
	}
	config, err := configuration.RESTClientGetter.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	installer := i.newBootstrapInstaller(store, config, options, selected, configuration, effective, kubeContext, imported)
	requested, err := installer.requested(ctx)
	if err != nil {
		return nil, err
	}
	if pending && (requested.IntentSHA256 != current.IntentSHA256 || requested.ChartSHA256 != current.ChartSHA256 ||
		requested.Mode != current.Mode || requested.Scope != current.Scope) {
		return nil, errors.New("bootstrap retry differs from the recorded install intent")
	}
	return installer, nil
}
