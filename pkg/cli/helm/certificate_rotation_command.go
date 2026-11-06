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
	"crypto/sha256"
	"encoding/pem"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func (i *Impl) runCertificateRotation(ctx context.Context, options ClusterOptions, kubeContext string, readOnly bool) error {
	if err := options.Certificates.Validate(); err != nil {
		return err
	}
	if options.ResetValues || len(options.Radius.SetArgs)+len(options.Radius.SetFileArgs)+len(options.Contour.SetArgs)+len(options.Contour.SetFileArgs) != 0 ||
		options.Radius.ChartPath != "" || options.Radius.Reinstall {
		return errors.New("CA rotation cannot be combined with release configuration changes")
	}
	flags := &genericclioptions.ConfigFlags{Namespace: &options.Radius.Namespace, Context: &kubeContext}
	configuration, err := initHelmConfig(flags)
	if err != nil {
		return err
	}
	release, err := i.Helm.RunHelmGet(configuration, radiusReleaseName)
	if err != nil {
		return err
	}
	if release == nil || release.Chart == nil || release.Chart.Metadata == nil ||
		release.Chart.Metadata.Annotations["radius.io/ca-rotation-protocol"] != "1" {
		return errors.New("upgrade the installed Radius chart to the guarded CA rotation protocol before starting rotation")
	}
	if release.Info == nil || strings.HasPrefix(string(release.Info.Status), "pending-") {
		return errors.New("a Helm operation is pending; recover it before changing CA trust")
	}
	effective := valuesutil.CoalesceTables(copyValues(release.Config), copyValues(release.Chart.Values))
	mode, err := resolveAuthzMode(nil, effective)
	if err != nil {
		return err
	}
	if mode == authz.ModeOff {
		return errors.New("CA rotation requires enabled component TLS and process evidence; enabling authorization is a separate upgrade")
	}
	config, err := configuration.RESTClientGetter.ToRESTConfig()
	if err != nil {
		return err
	}
	kube, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}
	store := &rotationStore{kube: kube, dynamic: dynamicClient, namespace: options.Radius.Namespace, release: radiusReleaseName, now: time.Now}
	ctx, cancel := context.WithTimeout(ctx, effectiveTimeout(options.Radius.Timeout))
	defer cancel()
	requested := rotationPhase(options.Certificates.RotationPhase)
	recovery := requested == rotationRecover
	journal, state, readErr := store.readJournal(ctx, recovery)
	if readErr != nil && !apierrors.IsNotFound(readErr) {
		return readErr
	}
	start := apierrors.IsNotFound(readErr) || state.Phase == rotationComplete && requested == rotationPublish
	if recovery {
		start = apierrors.IsNotFound(readErr) || options.Certificates.Specified() || !state.Recovery && state.Phase != rotationComplete
	}
	if start {
		if requested != rotationPublish && !recovery {
			return errors.New("no active CA rotation; start with --ca-rotation-phase publish")
		}
		if state.Phase == rotationComplete && !recovery {
			recorded, _, err := unstructured.NestedString(effective, "global", "rbac", "completedCARotation")
			if err != nil || recorded != state.ID {
				return errors.New("the previous rotation has not recorded its final Helm settings; retry its retire phase before starting another rotation")
			}
		}
		var importPlan *certificatePlan
		var target []byte
		var secret, targetMode string
		if recovery && state.Plan != nil && state.Phase != rotationComplete && !options.Certificates.Specified() {
			target, secret, targetMode = state.NewCA, state.Plan.SigningSecret, state.Plan.TargetMode
		} else {
			importPlan, target, secret, targetMode, err = i.rotationTarget(ctx, options, effective, kubeContext)
		}
		if err != nil {
			return err
		}
		oldCA, err := currentRotationComponentCA(ctx, store, effective, state, recovery)
		if err != nil {
			return err
		}
		id := uuid.NewString()
		var recovering *rotationState
		if recovery {
			recovering = &state
		}
		plan, err := planRotationResources(ctx, store, config, id, secret, targetMode, oldCA, target, importPlan, recovering)
		if err != nil {
			return err
		}
		if readOnly {
			return nil
		}
		if err := i.applyCertificatePlan(ctx, importPlan); err != nil {
			return err
		}
		signing, err := kube.CoreV1().Secrets(store.namespace).Get(ctx, secret, metav1.GetOptions{})
		if err != nil {
			return err
		}
		plan.SigningUID = signing.UID
		state = rotationState{NewCA: target, Plan: plan}
		if err := store.checkPlannedSigner(ctx, state); err != nil {
			return err
		}
		if recovery {
			i.logInfo("Recording explicit incident recovery: publish only the selected CA, reissue leaves, and replace consumers. Radius availability may be interrupted; old trust will not be restored.")
			state, err = store.beginRecovery(ctx, id, target, plan, journal)
		} else {
			state, err = store.beginPlanned(ctx, id, target, plan)
		}
		if err != nil {
			return err
		}
	} else {
		if state.Plan == nil {
			return errors.New("the public journal has no workload plan; it cannot authorize rotation")
		}
		if options.Certificates.Specified() {
			_, target, secret, _, err := i.rotationTarget(ctx, options, effective, kubeContext)
			if err != nil {
				return err
			}
			digest, _, err := mtls.TrustBundleDigest(target, time.Now())
			if err != nil || digest != state.Plan.TargetTrustSHA256 || secret != state.Plan.SigningSecret {
				return errors.New("target CA differs from the active rotation; resume without changing its target")
			}
		}
		if err := store.checkPlannedSigner(ctx, state); err != nil {
			return err
		}
		if readOnly {
			phase := state.Phase
			if requested == rotationRetire && phase == rotationReady {
				phase = rotationRetire
			}
			if !recovery && slices.Index(rotationPhases, phase) < slices.Index(rotationPhases, requested) {
				return fmt.Errorf("CA rotation is in phase %s; complete it before %s", state.Phase, requested)
			}
			return nil
		}
	}
	runner := rotationRunner{store: store, config: config, log: i.logInfo}
	if err := runner.run(ctx, state.ID, requested); err != nil {
		return err
	}
	_, state, err = store.read(ctx)
	if err != nil {
		return err
	}
	if state.Phase != rotationComplete {
		i.logInfo("CA rotation %s is now %s. Resume the next explicit phase; ordinary release changes remain guarded.", state.ID, state.Phase)
		return nil
	}
	// Record only the final certificate/proof settings. Reuse the current release
	// rather than upgrading software or changing unrelated user values.
	current, err := i.Helm.RunHelmGet(configuration, radiusReleaseName)
	if err != nil {
		return err
	}
	if current == nil || current.Chart == nil || current.Chart.Metadata == nil ||
		current.Chart.Metadata.Annotations["radius.io/ca-rotation-protocol"] != "1" {
		return errors.New("installed chart changed before final CA settings could be recorded")
	}
	values := map[string]any{}
	certificates := map[string]any{"mode": state.Plan.TargetMode}
	if state.Plan.TargetMode == "caSecret" {
		certificates["caSecret"], certificates["caBundle"] = state.Plan.SigningSecret, strings.TrimSpace(string(state.NewCA))
	}
	setCertificateValues(values, certificates)
	if err := unstructured.SetNestedField(values, state.ID, "global", "rbac", "completedCARotation"); err != nil {
		return err
	}
	if err := unstructured.SetNestedField(values, map[string]any{"enabled": true, "expectedTrustSHA256": state.Plan.TargetTrustSHA256},
		"global", "rbac", "startupProof"); err != nil {
		return err
	}
	if err := unstructured.SetNestedField(values, map[string]any{"enabled": true, "expectedTrustSHA256": state.Plan.TargetTrustSHA256},
		"de", "ucpMTLS", "startupProof"); err != nil {
		return err
	}
	upgrade := newUpgradeClient(configuration, store.namespace, true, true, options.Radius.Timeout, usesServerSideApply(configuration, radiusReleaseName))
	_, err = upgrade.RunWithContext(ctx, radiusReleaseName, current.Chart, values)
	if err != nil {
		return fmt.Errorf("CA retirement completed but recording Helm settings failed; retry the retire phase without rolling trust back: %w", err)
	}
	return nil
}

func (i *Impl) rotationTarget(ctx context.Context, options ClusterOptions, effective map[string]any, kubeContext string) (*certificatePlan, []byte, string, string, error) {
	values := copyValues(effective)
	selection := options
	selection.Certificates.RotationPhase = ""
	selection.RadiusReleaseValues = nil
	if options.Certificates.Specified() {
		setCertificateValues(values, nil)
	}
	plan, err := i.prepareCertificates(ctx, selection, nil, values, kubeContext, false)
	if err != nil {
		return nil, nil, "", "", err
	}
	mode := certificateMode(values)
	if mode == "caSecret" {
		settings := certificateValues(values)
		bundle, _ := settings["caBundle"].(string)
		secret, _ := settings["caSecret"].(string)
		return plan, []byte(bundle), secret, mode, nil
	}
	client, err := i.certificateClient(kubeContext)
	if err != nil {
		return nil, nil, "", "", err
	}
	secret, err := client.CoreV1().Secrets(options.Radius.Namespace).Get(ctx, "radius-ca", metav1.GetOptions{})
	if err != nil {
		return nil, nil, "", "", err
	}
	ca, err := validateSigningCA(secret.Data["tls.crt"], secret.Data["tls.key"], time.Now())
	if err != nil {
		return nil, nil, "", "", err
	}
	// Freeze the chosen self-signed root outside the renewing root Certificate.
	// cert-manager may renew radius-ca, but cannot silently change this issuer.
	fingerprint := sha256.Sum256(ca.Raw)
	name := fmt.Sprintf("radius-imported-ca-%x", fingerprint[:16])
	snapshot := &certificatePlan{client: client, namespace: options.Radius.Namespace}
	existing, err := client.CoreV1().Secrets(snapshot.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, nil, "", "", err
	}
	if err == nil {
		if existing.Type != corev1.SecretTypeTLS || existing.DeletionTimestamp != nil ||
			!bytes.Equal(existing.Data["tls.crt"], secret.Data["tls.crt"]) || !bytes.Equal(existing.Data["tls.key"], secret.Data["tls.key"]) {
			return nil, nil, "", "", errors.New("retained signing CA snapshot already exists with different content; it will not be overwritten")
		}
	} else {
		snapshot.secret = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: snapshot.namespace},
			Type: corev1.SecretTypeTLS, Data: map[string][]byte{"tls.crt": bytes.Clone(secret.Data["tls.crt"]), "tls.key": bytes.Clone(secret.Data["tls.key"])}}
	}
	return snapshot, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), name, "selfSigned", nil
}

func currentRotationComponentCA(ctx context.Context, store *rotationStore, values map[string]any, previous rotationState, recovery bool) ([]byte, error) {
	if previous.Phase == rotationComplete || recovery && previous.Plan != nil {
		return previous.NewCA, nil
	}
	if certificateMode(values) == "caSecret" {
		bundle, ok := certificateValues(values)["caBundle"].(string)
		if !ok {
			return nil, errors.New("installed dedicated public trust is missing")
		}
		return []byte(bundle), nil
	}
	secret, err := store.kube.CoreV1().Secrets(store.namespace).Get(ctx, "radius-ca", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if recovery {
		first, _ := pem.Decode(secret.Data["tls.crt"])
		if first == nil || first.Type != "CERTIFICATE" {
			return nil, errors.New("historical signing CA is missing")
		}
		public := pem.EncodeToMemory(first)
		if _, _, err := rotationHistoricalDigest(public); err != nil {
			return nil, err
		}
		return public, nil
	}
	ca, err := validateSigningCA(secret.Data["tls.crt"], secret.Data["tls.key"], store.now())
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), nil
}

func planRotationResources(ctx context.Context, store *rotationStore, config *rest.Config, id, secret, mode string, oldCA, target []byte, imported *certificatePlan, recovery *rotationState) (*rotationPlan, error) {
	oldDigest, roots, err := rotationHistoricalDigest(oldCA)
	if err != nil || len(roots) != 1 {
		return nil, errors.New("installed component trust must identify one valid dedicated CA")
	}
	digest, roots, err := mtls.TrustBundleDigest(target, store.now())
	if err != nil || len(roots) != 1 {
		return nil, errors.New("target trust must identify one valid dedicated CA")
	}
	plan := &rotationPlan{TargetMode: mode, TargetTrustSHA256: digest, TargetTrustMap: "radius-mtls-ca-" + digest[:32],
		OldComponentCA: oldCA, SigningSecret: secret, Issuer: "radius-ca-" + strings.ReplaceAll(id, "-", "")[:16],
		ServingSecret: "radius-aggregation-" + strings.ReplaceAll(id, "-", "")[:16]}
	signing, err := store.kube.CoreV1().Secrets(store.namespace).Get(ctx, secret, metav1.GetOptions{})
	if err == nil {
		plan.SigningUID = signing.UID
	} else if !apierrors.IsNotFound(err) || imported == nil || imported.secret == nil || imported.secret.Name != secret {
		return nil, fmt.Errorf("cannot inspect target signing Secret: %w", err)
	}
	api, err := store.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if err := store.checkAPIService(api); err != nil {
		return nil, err
	}
	aggregationCA, err := apiServiceBundle(api)
	if err != nil {
		return nil, err
	}
	journal, previous, err := store.readJournal(ctx, recovery != nil)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if journal == nil {
		if api.GetAnnotations()[rotationJournalUID] != "" || api.GetAnnotations()[rotationRevision] != "" {
			return nil, errors.New("restore the missing public journal before planning CA rotation")
		}
	} else {
		if api.GetUID() != previous.APIServiceUID ||
			api.GetAnnotations()[rotationJournalUID] != "" && api.GetAnnotations()[rotationJournalUID] != string(journal.UID) {
			return nil, errors.New("public journal or APIService identity changed before planning")
		}
		if recovery == nil && (previous.Phase != rotationComplete ||
			!bytes.Equal(aggregationCA, previous.NewCA) || api.GetAnnotations()[rotationRevision] != previous.revision() ||
			api.GetAnnotations()[rotationJournalUID] != string(journal.UID)) {
			return nil, errors.New("complete and reconcile the existing rotation before planning another")
		}
		if recovery != nil && (recovery.ID != previous.ID || recovery.Phase != previous.Phase) {
			return nil, errors.New("incident journal changed before planning; reread before retrying")
		}
	}
	selection := rotationState{SchemaVersion: 1, ID: id, Namespace: store.namespace, Release: store.release,
		APIServiceUID: api.GetUID(), Phase: rotationPublish, CreatedAt: store.now(), OldCA: aggregationCA, NewCA: target}
	if recovery != nil {
		if err := checkRecoveryTarget(target, *recovery); err != nil {
			return nil, err
		}
		selection.Phase, selection.Recovery = rotationRetire, true
		if recovery.ID != "" {
			selection.PreviousRevision = recovery.revision()
			if recovery.Phase == rotationComplete {
				if !bytes.Equal(aggregationCA, recovery.NewCA) {
					return nil, errors.New("completed public trust changed outside Radius")
				}
			} else {
				if !rotationHistoricalSubset(aggregationCA, bytes.Join([][]byte{recovery.OldCA, recovery.NewCA}, nil)) {
					return nil, errors.New("public trust changed outside the interrupted transaction")
				}
				selection.PriorRevisions = slices.Clone(recovery.PriorRevisions)
				if recovery.PreviousRevision != "" {
					selection.PriorRevisions = append(selection.PriorRevisions, recovery.PreviousRevision)
				}
			}
		}
		// In an interrupted rotation the target may already be the serving CA.
		// beginRecovery records the complete, bounded predecessor set.
		if recovery.ID != "" && recovery.Phase != rotationComplete {
			history := [][]byte{recovery.OldCA, recovery.NewCA, aggregationCA}
			if recovery.Plan != nil {
				history = append(history, recovery.Plan.OldComponentCA)
			}
			historical, err := rotationHistoricalRoots(history...)
			if err != nil {
				return nil, err
			}
			selection.OldCA = nil
			for _, hash := range slices.Sorted(maps.Keys(historical)) {
				selection.OldCA = append(selection.OldCA, historical[hash]...)
			}
		}
	}
	if err := selection.validate(store.now()); err != nil {
		return nil, err
	}
	service, err := store.kube.CoreV1().Services(store.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if !rotationOwned(service, store.namespace, store.release) || len(service.Spec.Selector) == 0 ||
		recovery == nil && service.Spec.Selector["pod-template-hash"] != "" {
		return nil, errors.New("rotation requires the owned UCP Service without a manually pinned rollout selector")
	}
	plan.ServiceUID, plan.ServiceSelector = service.UID, maps.Clone(service.Spec.Selector)
	if recovery != nil && recovery.Plan != nil {
		if service.UID != recovery.Plan.ServiceUID {
			return nil, errors.New("aggregation Service was replaced before incident recovery")
		}
		if err := checkRecoveryServiceSelector(service, *recovery); err != nil {
			return nil, err
		}
		plan.ServiceSelector = maps.Clone(recovery.Plan.ServiceSelector)
	}
	targets := []rotationConsumer{}
	for _, name := range rotationComponents {
		deployment, err := store.kube.AppsV1().Deployments(store.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		certificate, err := store.dynamic.Resource(rotationCertificateResource).Namespace(store.namespace).Get(ctx, name+"-cert", metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if !rotationOwned(deployment, store.namespace, store.release) || !rotationOwned(certificate, store.namespace, store.release) {
			return nil, errors.New("rotation requires Helm-owned component deployments and Certificates")
		}
		if recovery != nil && recovery.Plan != nil && recovery.Phase != rotationComplete {
			for _, previous := range recovery.Plan.Consumers {
				if previous.Name == name && (previous.UID != deployment.UID || previous.CertificateUID != certificate.GetUID()) {
					return nil, errors.New("an interrupted rotation workload was replaced before incident recovery")
				}
			}
		}
		issuer, _, err := unstructured.NestedString(certificate.Object, "spec", "issuerRef", "name")
		if err != nil || issuer == "" {
			return nil, errors.New("component Certificate issuer is missing")
		}
		containerName, portName := name, "metrics"
		if name == "bicep-de" {
			containerName, portName = "de", "bicep-de-api"
		}
		var port uint16
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if container.Name == containerName {
				for _, declared := range container.Ports {
					if declared.Name == portName && declared.ContainerPort > 0 && declared.ContainerPort <= 65535 {
						port = uint16(declared.ContainerPort)
					}
				}
			}
		}
		if port == 0 {
			return nil, fmt.Errorf("%s has no fixed process-evidence listener", name)
		}
		var leafDigest string
		if recovery == nil {
			leaf, err := store.kube.CoreV1().Secrets(store.namespace).Get(ctx, name+"-mtls", metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			leafDigest, err = rotationLeafDigest(leaf, oldCA, store.namespace, name, true, store.now())
			if err != nil {
				return nil, err
			}
		}
		plan.Consumers = append(plan.Consumers, rotationWorkload{Name: name, UID: deployment.UID, CertificateUID: certificate.GetUID(), OldIssuer: issuer, ProofPort: port})
		targets = append(targets, rotationConsumer{Name: name, DeploymentUID: deployment.UID, TrustSHA256: oldDigest, LeafSHA256: leafDigest, ProofPort: port})
		if name == "ucp" {
			plan.UCPMinReadySeconds = deployment.Spec.MinReadySeconds
			if deployment.Spec.ProgressDeadlineSeconds == nil {
				return nil, errors.New("UCP deployment has no API-defaulted progress deadline")
			}
			plan.UCPProgressDeadlineSeconds = *deployment.Spec.ProgressDeadlineSeconds
			if recovery != nil && recovery.Plan != nil {
				plan.UCPMinReadySeconds = recovery.Plan.UCPMinReadySeconds
				plan.UCPProgressDeadlineSeconds = recovery.Plan.UCPProgressDeadlineSeconds
			}
		}
	}
	// A create-only import has no server-assigned UID yet. Validate every other
	// field before creating it, then bind and revalidate the real UID afterward.
	validationPlan := *plan
	if validationPlan.SigningUID == "" {
		validationPlan.SigningUID = "pending-create"
	}
	selection.Plan = &validationPlan
	if err := selection.validate(store.now()); err != nil {
		return nil, err
	}
	if recovery != nil {
		return plan, nil
	}
	observer := rotationConsumerObserver{kube: store.kube, config: config, namespace: store.namespace, release: store.release, initial: true}
	if err := observer.verify(ctx, targets); err != nil {
		return nil, fmt.Errorf("verify compatible process-evidence images before starting rotation: %w", err)
	}
	ids, _, err := store.aggregationPods(ctx, rotationState{Plan: plan}, false)
	if err != nil {
		return nil, err
	}
	if err := observeAggregatedPod(ctx, config, ids); err != nil {
		return nil, fmt.Errorf("verify the UCP aggregation identity capability before starting rotation: %w", err)
	}
	return plan, nil
}
