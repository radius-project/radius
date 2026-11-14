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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	certificatePrepare        rotationPhase = "prepare"
	certificateExecutionOwner               = "radius.io/certificate-lifecycle"
)

// Intent contains public identities and hashes, never private imports, arbitrary
// operator issuer configurations or Helm values that might contain credentials.
type certificateLifecycleExecution struct {
	NamespaceUID             types.UID                   `json:"namespaceUID"`
	ChartSHA256              string                      `json:"chartSHA256"`
	BaselineValuesSHA256     string                      `json:"baselineValuesSHA256"`
	TargetValuesSHA256       string                      `json:"targetValuesSHA256"`
	BaselineVersion          int                         `json:"baselineVersion"`
	Resources                []certificateResourceIntent `json:"resources"`
	PendingSource            *certificateSourceIntent    `json:"pendingSource,omitempty"`
	PendingResource          string                      `json:"pendingResource,omitempty"`
	Mutations                []certificateMutationIntent `json:"mutations,omitempty"`
	ServingSecret            string                      `json:"servingSecret,omitempty"`
	PreviousServingSecret    string                      `json:"previousServingSecret,omitempty"`
	UnchangedAggregationLeaf *aggregationBootstrapPin    `json:"unchangedAggregationLeaf,omitempty"`
	FinalReleaseVersion      int                         `json:"finalReleaseVersion,omitempty"`
	CAInjectorSHA256         string                      `json:"caInjectorSHA256,omitempty"`
}

type certificateResourceIntent struct {
	Kind     string                   `json:"kind"`
	Name     string                   `json:"name"`
	Plane    string                   `json:"plane"`
	Spec     map[string]any           `json:"spec"`
	Before   *aggregationBootstrapPin `json:"before,omitempty"`
	Observed *aggregationBootstrapPin `json:"observed,omitempty"`
}

type certificateSourceIntent struct {
	Plane    string                     `json:"plane"`
	Before   certificateTrustProjection `json:"before"`
	Bundle   []byte                     `json:"bundle"`
	Revision string                     `json:"revision"`
}

func (state certificateLifecycle) validateTargetSnapshot(name string, plane certificateLifecyclePlane) error {
	if plane.TargetIssuer.UID != "" || state.Execution == nil {
		return plane.validateSnapshots(state.Namespace, state.Mode)
	}
	if !plane.Affected || state.Mode == "issuerRef" ||
		!slices.Contains([]rotationPhase{rotationPublish, rotationAdopt, rotationIssue}, state.Phase) {
		return errors.New("target issuer has not been observed")
	}
	if err := plane.validatePreviousSnapshot(state.Namespace); err != nil {
		return err
	}
	for _, intent := range state.Execution.Resources {
		if intent.Plane != name || intent.Kind != "Issuer" || intent.Name != plane.Issuer.Name {
			continue
		}
		ca, ok := intent.Spec["ca"].(map[string]any)
		if !ok || plane.Signer == nil || ca["secretName"] != plane.Signer.Name || intent.Observed != nil {
			return errors.New("unobserved issuer must match its recorded target signer configuration")
		}
		if intent.Before != nil && (intent.Before.UID != plane.Previous.Issuer.UID ||
			intent.Before.ResourceVersion != plane.Previous.Issuer.ResourceVersion ||
			intent.Before.SpecSHA256 != plane.Previous.Issuer.SpecSHA256 || plane.Issuer != plane.Previous.Issuer.Reference) {
			return errors.New("issuer mutation does not preserve the exact previous configuration")
		}
		return nil
	}
	return errors.New("unobserved issuer has no durable staged configuration intent")
}

func (state certificateLifecycle) validateExecution() error {
	plan := state.Execution
	if plan.NamespaceUID == "" || !validCertificateHash(plan.ChartSHA256) ||
		!validCertificateHash(plan.BaselineValuesSHA256) || !validCertificateHash(plan.TargetValuesSHA256) ||
		plan.BaselineVersion < 1 || len(plan.Resources) > 8 {
		return errors.New("certificate execution requires namespace, chart and installed/target history pins")
	}
	if err := state.validateMutations(); err != nil {
		return err
	}
	if plan.FinalReleaseVersion != 0 && (state.Phase != rotationComplete || plan.FinalReleaseVersion <= plan.BaselineVersion) {
		return errors.New("final release receipt requires a completed lifecycle and a newer Helm revision")
	}
	if plan.CAInjectorSHA256 != "" && (!state.Planes["aggregation"].Affected || !validCertificateHash(plan.CAInjectorSHA256)) {
		return errors.New("cainjector configuration pin must belong to an affected aggregation plane")
	}
	if plan.ServingSecret != "" || plan.PreviousServingSecret != "" {
		if state.Planes["aggregation"].Affected == (plan.ServingSecret == plan.PreviousServingSecret) ||
			len(validation.IsDNS1123Subdomain(plan.ServingSecret)) != 0 ||
			len(validation.IsDNS1123Subdomain(plan.PreviousServingSecret)) != 0 {
			return errors.New("aggregation reissuance requires distinct valid Secret names; an unaffected plane must preserve its Secret")
		}
	}
	if pin := plan.UnchangedAggregationLeaf; pin != nil {
		if state.Planes["aggregation"].Affected || pin.UID == "" || pin.ResourceVersion == "" || !validCertificateHash(pin.SpecSHA256) {
			return errors.New("unchanged aggregation leaf requires its original pinned specification")
		}
	}
	seen := map[string]bool{}
	for _, resource := range plan.Resources {
		key := resource.Kind + "/" + resource.Name
		if seen[key] || len(validation.IsDNS1123Subdomain(resource.Name)) != 0 ||
			!slices.Contains([]string{"internal", "aggregation"}, resource.Plane) ||
			!state.Planes[resource.Plane].Affected {
			return errors.New("planned certificate resource has invalid, duplicate or unaffected identity")
		}
		seen[key] = true
		if err := resource.validateSpec(); err != nil {
			return err
		}
		for _, pin := range []*aggregationBootstrapPin{resource.Before, resource.Observed} {
			if pin != nil && (pin.UID == "" || pin.ResourceVersion == "" || !validCertificateHash(pin.SpecSHA256)) {
				return errors.New("planned resource observations require real identity and configuration pins")
			}
		}
		if resource.Before != nil && resource.Observed != nil && resource.Before.UID != resource.Observed.UID {
			return errors.New("planned resource mutation cannot replace its original identity")
		}
		if state.Phase == rotationComplete && resource.Observed == nil {
			return errors.New("completed lifecycle has an unobserved planned resource")
		}
	}
	if plan.PendingResource != "" && (state.Phase != rotationIssue || !seen[plan.PendingResource]) {
		return errors.New("pending issuer mutation must select a recorded resource in the issue phase")
	}
	if state.Phase == rotationComplete {
		for _, plane := range state.Planes {
			if plane.Affected && (plane.Source.JournalUID == "" || plane.Source.Revision != state.ID+"/complete") {
				return errors.New("completed execution must preserve each affected plane's final source publication")
			}
		}
	}
	if pending := plan.PendingSource; pending != nil {
		plane, found := state.Planes[pending.Plane]
		if !found || !plane.Affected || !reflect.DeepEqual(pending.Before, plane.Source) ||
			!slices.Contains([]rotationPhase{certificatePrepare, rotationRetire}, state.Phase) ||
			!slices.Contains([]string{state.ID + "/publish", state.ID + "/retire", state.ID + "/complete"}, pending.Revision) {
			return errors.New("pending trust write is not bound to the exact active source and phase")
		}
		want := plane.NewCA
		if state.Phase == certificatePrepare {
			if pending.Revision != state.ID+"/publish" {
				return errors.New("preparation cannot record retirement")
			}
			want = append(bytes.Clone(plane.OldCA), plane.NewCA...)
		}
		if len(plane.NewCA) == 0 || !bytes.Equal(want, pending.Bundle) {
			return errors.New("pending trust write does not contain the exact intended bytes")
		}
	}
	return nil
}

func (intent certificateResourceIntent) validateSpec() error {
	switch intent.Kind {
	case "Issuer":
		if len(intent.Spec) != 1 {
			return errors.New("planned local issuer must contain only a selfSigned or CA configuration")
		}
		if value, ok := intent.Spec["selfSigned"].(map[string]any); ok && len(value) == 0 {
			return nil
		}
		ca, ok := intent.Spec["ca"].(map[string]any)
		name, valid := ca["secretName"].(string)
		if !ok || !valid || len(ca) != 1 || validateCASecretName(name) != nil {
			return errors.New("planned local issuer must name exactly one signing Secret")
		}
	case "Certificate":
		if intent.Before != nil {
			return errors.New("new root intent cannot replace a pre-existing Certificate")
		}
		for key := range intent.Spec {
			if !slices.Contains([]string{"isCA", "commonName", "secretName", "duration", "renewBefore", "privateKey", "issuerRef"}, key) {
				return errors.New("planned root contains unsupported configuration")
			}
		}
		if intent.Spec["isCA"] != true || intent.Spec["secretName"] != intent.Name || intent.Spec["commonName"] != intent.Name {
			return errors.New("planned root must be a dedicated selfSigned CA")
		}
		key, keyOK := intent.Spec["privateKey"].(map[string]any)
		issuer, issuerOK := intent.Spec["issuerRef"].(map[string]any)
		if !keyOK || !issuerOK || len(key) != 3 || key["algorithm"] != "ECDSA" ||
			key["rotationPolicy"] != "Never" || fmt.Sprint(key["size"]) != "256" ||
			len(issuer) != 3 || issuer["name"] != RadiusBootstrapIssuerName ||
			issuer["group"] != "cert-manager.io" || issuer["kind"] != "Issuer" {
			return errors.New("planned root must preserve the selected P256 bootstrap profile")
		}
		for _, field := range []string{"duration", "renewBefore"} {
			if value, ok := intent.Spec[field].(string); !ok || value == "" {
				return errors.New("planned root requires explicit validity and renewal settings")
			}
		}
	default:
		return errors.New("only dedicated roots and local issuers may be staged as certificate resources")
	}
	return nil
}

func (intent certificateResourceIntent) existingBootstrap() bool {
	value, ok := intent.Spec["selfSigned"].(map[string]any)
	return intent.Before != nil && intent.Kind == "Issuer" && intent.Name == RadiusBootstrapIssuerName &&
		len(intent.Spec) == 1 && ok && len(value) == 0
}

func (state certificateLifecycle) validatePreparingPlane(name string, plane certificateLifecyclePlane, now time.Time) error {
	if plane.Previous == nil || plane.Source.UID != plane.Previous.Source.UID {
		return errors.New("preparation requires an observed previous configuration")
	}
	// Validate the historical observation independently of any unissued target.
	before := plane
	before.Affected = false
	before.Source = plane.Previous.Source
	before.NewCA = plane.OldCA
	before.Issuer, before.TargetIssuer = plane.Previous.Issuer.Reference, plane.Previous.Issuer
	before.Signer = plane.Previous.Signer
	if err := before.validate(name, rotationComplete, now); err != nil {
		return err
	}
	if err := before.validateSnapshots(state.Namespace, plane.Previous.Mode); err != nil {
		return err
	}
	if !plane.Affected {
		if !reflect.DeepEqual(plane, before) {
			return errors.New("preparation cannot change an unaffected plane")
		}
		return nil
	}
	if plane.Issuer.Group != "cert-manager.io" {
		return errors.New("planned issuer has an unsupported group")
	}
	if _, err := parseCertificateIssuer(plane.Issuer.Kind + "/" + plane.Issuer.Name); err != nil {
		return err
	}
	if state.Mode == "issuerRef" && plane.Issuer == plane.Previous.Issuer.Reference {
		return errors.New("issuerRef replacement requires a distinct staged issuer")
	}
	if len(plane.NewCA) > 0 {
		next, err := validatePublicCA(plane.NewCA, now)
		if err != nil {
			return err
		}
		old, err := parseCertificateBundlePEM(plane.OldCA)
		if err != nil || len(old) != 1 || bytes.Equal(next.Raw, old[0].Raw) {
			return errors.New("prepared target must replace the dedicated anchor")
		}
		if plane.Signer != nil && plane.Signer.SHA256 != fmt.Sprintf("%x", sha256.Sum256(next.Raw)) {
			return errors.New("prepared target signer differs from the dedicated anchor")
		}
	}
	if plane.TargetIssuer.UID != "" {
		if err := plane.TargetIssuer.validate(state.Namespace); err != nil {
			return err
		}
		if plane.TargetIssuer.Reference != plane.Issuer {
			return errors.New("prepared target issuer observation differs from its intent")
		}
	}
	if !reflect.DeepEqual(plane.Source, plane.Previous.Source) {
		overlap := append(bytes.Clone(plane.OldCA), plane.NewCA...)
		if len(plane.NewCA) == 0 || !bytes.Equal(plane.Source.Bundle, overlap) ||
			plane.Source.SHA256 != fmt.Sprintf("%x", sha256.Sum256(overlap)) ||
			plane.Source.ResourceVersion == "" || plane.Source.Revision != state.ID+"/publish" || plane.Source.JournalUID == "" {
			return errors.New("preparation changed its source without an exact observed overlap publication")
		}
	}
	return nil
}

// certificateExecutionWriter is a per-operation journal writer, not a way to
// acquire protected observations. Its caller must resolve that gate before
// creating a Lease or invoking any mutator.
type certificateExecutionWriter struct {
	store          *rotationStore
	operation      *identityOperation
	journal        *corev1.ConfigMap
	state          certificateLifecycle
	observeLoaded  func(context.Context, certificateLifecycle) error
	observeServing func(context.Context, certificateLifecycle) error
}

func (w *certificateExecutionWriter) check(ctx context.Context) error {
	if w.state.Execution == nil {
		return errors.New("certificate writes require a durable execution plan")
	}
	if err := w.operation.checkProviderJournal(ctx, w.store.release, nil); err != nil {
		return err
	}
	if err := w.state.validate(w.store.now()); err != nil {
		return err
	}
	namespace, err := w.store.kube.CoreV1().Namespaces().Get(ctx, w.store.namespace, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if namespace.UID != w.state.Execution.NamespaceUID || namespace.DeletionTimestamp != nil {
		return errors.New("certificate operation namespace changed")
	}
	_, initial, err := readInitialCertificateJournal(ctx, w.store)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err == nil && initial.Phase != "complete" || apierrors.IsNotFound(err) && namespace.Annotations[initialCertificateIntent] != "" {
		return errors.New("complete initial certificate bootstrap before rotating")
	}
	if _, found, err := w.store.bootstrapIntent(ctx); err != nil {
		return err
	} else if found {
		return errors.New("legacy bootstrap cannot be resumed by the token/CSI executor")
	}
	if w.journal == nil {
		if _, err := w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Get(ctx, rotationJournalName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			if err != nil {
				return err
			}
			return errors.New("a certificate lifecycle journal already exists")
		}
	} else {
		current, err := w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Get(ctx, w.journal.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if current.UID != w.journal.UID || current.ResourceVersion != w.journal.ResourceVersion || !reflect.DeepEqual(current.Data, w.journal.Data) {
			return errors.New("certificate execution journal changed")
		}
	}
	if err := w.checkMaterial(ctx); err != nil {
		return err
	}
	return w.operation.check(ctx)
}

func (w *certificateExecutionWriter) checkMaterial(ctx context.Context) error {
	if w.state.Phase != rotationComplete {
		if pin := w.state.Execution.UnchangedAggregationLeaf; pin != nil {
			leaf, err := w.store.dynamic.Resource(rotationCertificateResource).Namespace(w.store.namespace).Get(ctx, "ucp-cert", metav1.GetOptions{})
			if err != nil {
				return err
			}
			actual, err := bootstrapResourcePin(leaf)
			if err != nil || actual.UID != pin.UID || actual.SpecSHA256 != pin.SpecSHA256 ||
				!rotationOwned(leaf, w.store.namespace, w.store.release) || len(leaf.GetOwnerReferences()) != 0 || leaf.GetDeletionTimestamp() != nil {
				return errors.New("unaffected aggregation leaf changed during internal CA rotation")
			}
		}
		if w.state.Execution.CAInjectorSHA256 != "" {
			instances, err := inspectCAInjectorPrerequisites(ctx, w.store.kube, w.store.namespace)
			if err != nil {
				return err
			}
			digest, err := bootstrapDigest(instances)
			if err != nil || digest != w.state.Execution.CAInjectorSHA256 {
				return errors.New("cainjector ownership, readiness or configuration changed during execution")
			}
		}
		if err := w.checkMutations(ctx); err != nil {
			return err
		}
	}
	for name, plane := range w.state.Planes {
		switched := plane.TargetIssuer.UID != "" && plane.TargetIssuer.Reference == plane.Previous.Issuer.Reference &&
			plane.TargetIssuer.SpecSHA256 != plane.Previous.Issuer.SpecSHA256
		pending := w.pendingIssuer(name)
		if !switched && w.state.Phase != rotationComplete {
			if pending != nil && pending.Name == plane.Previous.Issuer.Reference.Name {
				if _, err := w.readIssuerMutation(ctx, *pending); err != nil {
					return err
				}
			} else if err := w.store.checkCertificateIssuerSnapshot(ctx, plane.Previous.Issuer, w.state.Phase); err != nil {
				return err
			}
		}
		if w.state.Phase != rotationRetire && w.state.Phase != rotationComplete {
			if err := w.store.checkCertificateSignerSnapshot(ctx, plane.Previous.Signer, w.state.Phase); err != nil {
				return err
			}
		}
		if plane.TargetIssuer.UID != "" {
			if err := w.store.checkCertificateIssuerSnapshot(ctx, plane.TargetIssuer, w.state.Phase); err != nil {
				return err
			}
		}
		if err := w.store.checkCertificateSignerSnapshot(ctx, plane.Signer, w.state.Phase); err != nil {
			return err
		}
		if err := w.checkSource(ctx, name, plane); err != nil {
			return err
		}
	}
	for _, intent := range w.state.Execution.Resources {
		if intent.Observed == nil {
			continue
		}
		object, err := w.store.dynamic.Resource(certificateIntentResource(intent.Kind)).
			Namespace(w.store.namespace).Get(ctx, intent.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		pin, err := bootstrapResourcePin(object)
		if err != nil {
			return err
		}
		if pin.UID != intent.Observed.UID || pin.SpecSHA256 != intent.Observed.SpecSHA256 ||
			!rotationOwned(object, w.store.namespace, w.store.release) ||
			!intent.existingBootstrap() && object.GetAnnotations()[certificateExecutionOwner] != string(w.journal.UID) ||
			len(object.GetOwnerReferences()) != 0 {
			return errors.New("recorded staged resource identity or configuration changed")
		}
	}
	if w.state.APIServiceUID != "" {
		api, err := w.store.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if err := w.store.checkLifecycleAPIService(api); err != nil {
			return err
		}
		if api.GetUID() != w.state.APIServiceUID {
			return errors.New("aggregation APIService identity changed during execution")
		}
	}
	return ctx.Err()
}

func (w *certificateExecutionWriter) pendingIssuer(plane string) *certificateResourceIntent {
	for _, intent := range w.state.Execution.Resources {
		if intent.Plane == plane && intent.Kind == "Issuer" &&
			w.state.Execution.PendingResource == intent.Kind+"/"+intent.Name {
			return &intent
		}
	}
	return nil
}

func (w *certificateExecutionWriter) readIssuerMutation(ctx context.Context, intent certificateResourceIntent) (*unstructured.Unstructured, error) {
	object, err := w.store.dynamic.Resource(rotationIssuerResource).Namespace(w.store.namespace).Get(ctx, intent.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	pin, err := bootstrapResourcePin(object)
	if err != nil {
		return nil, err
	}
	if intent.Before != nil && pin.UID != intent.Before.UID ||
		!rotationOwned(object, w.store.namespace, w.store.release) || len(object.GetOwnerReferences()) != 0 {
		return nil, errors.New("issuer mutation lost its original owned identity")
	}
	digest, err := bootstrapDigest(intent.Spec)
	if err != nil {
		return nil, err
	}
	after := pin.SpecSHA256 == digest && object.GetAnnotations()[certificateExecutionOwner] == string(w.journal.UID) &&
		object.GetAnnotations()["helm.sh/resource-policy"] == "keep"
	before := intent.Before != nil && pin.SpecSHA256 == intent.Before.SpecSHA256 && pin.ResourceVersion == intent.Before.ResourceVersion
	if !before && !after {
		return nil, errors.New("issuer differs from both its exact before snapshot and recorded mutation")
	}
	return object, nil
}

func (w *certificateExecutionWriter) applyIssuer(ctx context.Context, index int) error {
	if w.observeLoaded == nil {
		return errCertificateObserverUnavailable
	}
	if w.state.Phase != rotationIssue || w.journal == nil || index < 0 || index >= len(w.state.Execution.Resources) {
		return errors.New("issuer switching requires the explicit issue phase and a recorded resource")
	}
	intent := w.state.Execution.Resources[index]
	plane := w.state.Planes[intent.Plane]
	if intent.Kind != "Issuer" || intent.Name != plane.Issuer.Name || !plane.Affected || plane.Signer == nil {
		return errors.New("issuer mutation does not select the affected plane's local signer")
	}
	ca, ok := intent.Spec["ca"].(map[string]any)
	if !ok || ca["secretName"] != plane.Signer.Name {
		return errors.New("issuer mutation does not match the pinned target signer")
	}
	if err := w.check(ctx); err != nil {
		return err
	}
	if intent.Observed != nil {
		return nil
	}
	if err := w.observeLoaded(ctx, w.state); err != nil {
		return err
	}
	key := intent.Kind + "/" + intent.Name
	if w.state.Execution.PendingResource == "" {
		next, err := cloneCertificateLifecycle(w.state)
		if err != nil {
			return err
		}
		next.Execution.PendingResource = key
		if err := w.save(ctx, next); err != nil {
			return err
		}
	} else if w.state.Execution.PendingResource != key {
		return errors.New("finish the recorded issuer mutation first")
	}
	object, err := w.readIssuerMutation(ctx, intent)
	missing := apierrors.IsNotFound(err) && intent.Before == nil
	if err != nil && !missing {
		return err
	}
	desiredDigest, err := bootstrapDigest(intent.Spec)
	if err != nil {
		return err
	}
	actualDigest := ""
	if !missing {
		actualDigest, err = rotationIssuerSpecDigest(object)
		if err != nil {
			return err
		}
	}
	if missing || actualDigest != desiredDigest || object.GetAnnotations()[certificateExecutionOwner] != string(w.journal.UID) {
		// Approval is not a persisted boolean. Re-observe the actual loaded
		// authorities immediately before each conditional issuance change.
		if err := w.observeLoaded(ctx, w.state); err != nil {
			return err
		}
		if err := w.check(ctx); err != nil {
			return err
		}
		if missing {
			object = &unstructured.Unstructured{Object: map[string]any{"apiVersion": "cert-manager.io/v1", "kind": "Issuer"}}
			object.SetName(intent.Name)
			object.SetNamespace(w.store.namespace)
			object.SetLabels(map[string]string{helmManagedByLabel: helmManagedByValue})
			object.SetAnnotations(map[string]string{helmReleaseNameAnnotation: w.store.release, helmReleaseNamespaceAnnotation: w.store.namespace})
		}
		object.Object["spec"] = intent.Spec
		annotations := maps.Clone(object.GetAnnotations())
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[certificateExecutionOwner], annotations["helm.sh/resource-policy"] = string(w.journal.UID), "keep"
		object.SetAnnotations(annotations)
		client := w.store.dynamic.Resource(rotationIssuerResource).Namespace(w.store.namespace)
		if missing {
			object, err = client.Create(ctx, object, metav1.CreateOptions{FieldManager: FieldManager})
		} else {
			object, err = client.Update(ctx, object, metav1.UpdateOptions{FieldManager: FieldManager})
		}
		if err != nil {
			return err
		}
	}
	object, err = w.waitIssuerReady(ctx, intent)
	if err != nil {
		return err
	}
	pin, err := bootstrapResourcePin(object)
	if err != nil {
		return err
	}
	if intent.Before != nil && pin.UID != intent.Before.UID || pin.SpecSHA256 != desiredDigest {
		return errors.New("issuer write did not retain the selected identity and configuration")
	}
	next, err := cloneCertificateLifecycle(w.state)
	if err != nil {
		return err
	}
	updated := next.Planes[intent.Plane]
	updated.TargetIssuer = certificateIssuerSnapshot{Reference: updated.Issuer, Namespace: w.store.namespace,
		UID: pin.UID, ResourceVersion: pin.ResourceVersion, SpecSHA256: pin.SpecSHA256, SigningSecret: updated.Signer.Name}
	next.Planes[intent.Plane] = updated
	next.Execution.Resources[index].Observed = &pin
	next.Execution.PendingResource = ""
	return w.save(ctx, next)
}

func (w *certificateExecutionWriter) waitIssuerReady(ctx context.Context, intent certificateResourceIntent) (*unstructured.Unstructured, error) {
	var object *unstructured.Unstructured
	err := wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, effectiveTimeout(0), true, func(ctx context.Context) (bool, error) {
		if err := w.check(ctx); err != nil {
			return false, err
		}
		var err error
		object, err = w.readIssuerMutation(ctx, intent)
		if err != nil {
			return false, err
		}
		return certificateResourceReady(object)
	})
	if err != nil {
		return nil, fmt.Errorf("waiting for selected issuer %s: %w", intent.Name, err)
	}
	return object, nil
}

func certificateResourceReady(object *unstructured.Unstructured) (bool, error) {
	if object.GetGeneration() < 1 || object.GetDeletionTimestamp() != nil {
		return false, errors.New("certificate resource lacks a live generation")
	}
	conditions, _, err := unstructured.NestedSlice(object.Object, "status", "conditions")
	if err != nil {
		return false, err
	}
	for _, entry := range conditions {
		condition, ok := entry.(map[string]any)
		if !ok {
			return false, errors.New("certificate resource has malformed conditions")
		}
		if condition["type"] == "Ready" {
			return condition["status"] == "True" && condition["observedGeneration"] == object.GetGeneration(), nil
		}
	}
	return false, nil
}

func (w *certificateExecutionWriter) readSource(ctx context.Context, name string) (metav1.Object, []byte, error) {
	if name == "aggregation" {
		object, err := w.store.kube.CoreV1().Secrets(w.store.namespace).Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
		if err != nil {
			return nil, nil, err
		}
		if err := publicAggregationSource(object, w.store.namespace); err != nil {
			return nil, nil, err
		}
		return object, object.Data[caCertificateKey], nil
	}
	object, err := w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Get(ctx, "radius-mtls-ca", metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}
	if !rotationOwned(object, w.store.namespace, w.store.release) || len(object.OwnerReferences) != 0 ||
		len(object.Data) != 1 || len(object.BinaryData) != 0 || object.DeletionTimestamp != nil {
		return nil, nil, errors.New("internal public source ownership or contents changed")
	}
	return object, []byte(object.Data[caCertificateKey]), nil
}

func (w *certificateExecutionWriter) checkSource(ctx context.Context, name string, plane certificateLifecyclePlane) error {
	object, bundle, err := w.readSource(ctx, name)
	if err != nil {
		return err
	}
	if object.GetUID() != plane.Source.UID || object.GetResourceVersion() == "" {
		return errors.New("public source identity changed during execution")
	}
	if object.GetAnnotations()["helm.sh/resource-policy"] != "keep" {
		return errors.New("public source must retain its keep policy")
	}
	if plane.Affected && plane.Source.JournalUID != "" && (w.journal == nil || plane.Source.JournalUID != w.journal.UID) {
		return errors.New("affected source belongs to another execution journal")
	}
	expected := plane.Source
	mode := w.state.Mode
	if expected.Revision == plane.Previous.Source.Revision && expected.JournalUID == plane.Previous.Source.JournalUID {
		mode = plane.Previous.Mode
	}
	if pending := w.state.Execution.PendingSource; pending != nil && pending.Plane == name &&
		bytes.Equal(bundle, pending.Bundle) &&
		object.GetAnnotations()[rotationJournalUID] == string(w.journal.UID) &&
		object.GetAnnotations()[rotationRevision] == pending.Revision &&
		object.GetAnnotations()["radius.io/certificate-mode"] == w.state.Mode {
		return nil
	}
	if !bytes.Equal(bundle, expected.Bundle) || object.GetAnnotations()[rotationJournalUID] != string(expected.JournalUID) ||
		object.GetAnnotations()[rotationRevision] != expected.Revision ||
		object.GetAnnotations()["radius.io/certificate-mode"] != mode ||
		w.state.Phase != rotationComplete && object.GetResourceVersion() != expected.ResourceVersion {
		return errors.New("public source differs from the exact observed or pending execution snapshot")
	}
	return nil
}

func (w *certificateExecutionWriter) save(ctx context.Context, next certificateLifecycle) error {
	if err := w.check(ctx); err != nil {
		return err
	}
	if err := next.validate(w.store.now()); err != nil {
		return err
	}
	if err := w.checkNext(ctx, next); err != nil {
		return err
	}
	inspector := *w
	inspector.state = next
	if err := inspector.checkMaterial(ctx); err != nil {
		return fmt.Errorf("cannot record an unobserved execution result: %w", err)
	}
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if len(data) > maxRotationBytes {
		return errors.New("certificate execution journal exceeds its bounded size")
	}
	client := w.store.kube.CoreV1().ConfigMaps(w.store.namespace)
	if err := w.operation.check(ctx); err != nil {
		return err
	}
	var saved *corev1.ConfigMap
	if w.journal == nil {
		saved, err = client.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: rotationJournalName, Namespace: w.store.namespace, Labels: map[string]string{rotationOwner: w.store.release},
		}, Data: map[string]string{rotationJournalKey: string(data)}}, metav1.CreateOptions{FieldManager: FieldManager})
	} else {
		copy := w.journal.DeepCopy()
		copy.Data = map[string]string{rotationJournalKey: string(data)}
		saved, err = client.Update(ctx, copy, metav1.UpdateOptions{FieldManager: FieldManager})
	}
	if err != nil {
		return err
	}
	w.journal, w.state = saved, next
	return ctx.Err()
}

func (w *certificateExecutionWriter) checkNext(ctx context.Context, next certificateLifecycle) error {
	before := w.state
	if before.ID != next.ID || before.Namespace != next.Namespace || before.Release != next.Release ||
		before.Mode != next.Mode || before.Provider != next.Provider || before.APIServiceUID != next.APIServiceUID ||
		!before.CreatedAt.Equal(next.CreatedAt) || next.Execution == nil {
		return errors.New("execution cannot change its recorded transaction or installation identity")
	}
	a, b := *before.Execution, *next.Execution
	a.Resources, a.PendingSource, a.PendingResource = nil, nil, ""
	b.Resources, b.PendingSource, b.PendingResource = nil, nil, ""
	a.Mutations, b.Mutations = nil, nil
	if a.FinalReleaseVersion != b.FinalReleaseVersion &&
		(before.Phase != rotationComplete || next.Phase != rotationComplete || a.FinalReleaseVersion != 0) {
		return errors.New("only completed trust may record its final Helm release once")
	}
	a.FinalReleaseVersion, b.FinalReleaseVersion = 0, 0
	if err := checkNextCertificateMutations(before, next); err != nil {
		return err
	}
	if !reflect.DeepEqual(a, b) || len(before.Execution.Resources) != len(next.Execution.Resources) {
		return errors.New("execution cannot change its selected chart, target values or resource intent")
	}
	for index, intent := range before.Execution.Resources {
		target := next.Execution.Resources[index]
		intent.Observed, target.Observed = nil, nil
		oldHash, err := bootstrapDigest(intent)
		if err != nil {
			return err
		}
		newHash, err := bootstrapDigest(target)
		if err != nil || oldHash != newHash {
			return errors.New("execution cannot rewrite a recorded resource intent")
		}
	}
	for name, plane := range before.Planes {
		target, found := next.Planes[name]
		if !found || plane.Affected != target.Affected || plane.Issuer != target.Issuer ||
			!bytes.Equal(plane.OldCA, target.OldCA) || !reflect.DeepEqual(plane.Previous, target.Previous) ||
			len(plane.NewCA) != 0 && !bytes.Equal(plane.NewCA, target.NewCA) ||
			plane.Signer != nil && !reflect.DeepEqual(plane.Signer, target.Signer) ||
			plane.TargetIssuer.UID != "" && plane.TargetIssuer != target.TargetIssuer {
			return errors.New("execution cannot rewrite its before snapshot or an observed target")
		}
	}
	if before.Phase == next.Phase {
		return nil
	}
	phases := append([]rotationPhase{certificatePrepare}, rotationPhases...)
	if slices.Index(phases, next.Phase) != slices.Index(phases, before.Phase)+1 {
		return errors.New("execution must finish each durable phase in order")
	}
	if next.Phase != rotationPublish {
		if err := w.checkAggregationTrust(ctx); err != nil {
			return err
		}
	}
	if slices.Contains([]rotationPhase{rotationIssue, rotationReady, rotationRetire, rotationComplete}, next.Phase) {
		if w.observeLoaded == nil {
			return errCertificateObserverUnavailable
		}
		if err := w.observeLoaded(ctx, before); err != nil {
			return err
		}
	}
	if slices.Contains([]rotationPhase{rotationReady, rotationRetire, rotationComplete}, next.Phase) {
		if w.observeServing == nil {
			return errors.New("certificate phase requires exact serving-pod observation, separately from outbound trust")
		}
		if err := w.observeServing(ctx, before); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func certificateIntentResource(kind string) schema.GroupVersionResource {
	if kind == "Certificate" {
		return rotationCertificateResource
	}
	return rotationIssuerResource
}

func (w *certificateExecutionWriter) ensureResource(ctx context.Context, index int) error {
	if w.journal == nil || index < 0 || index >= len(w.state.Execution.Resources) {
		return errors.New("resource creation requires recorded intent")
	}
	if err := w.check(ctx); err != nil {
		return err
	}
	intent := w.state.Execution.Resources[index]
	if intent.Kind == "Issuer" && intent.Spec["ca"] != nil {
		return errors.New("signing issuer changes must pass the loaded-trust barrier in applyIssuer")
	}
	if intent.Before != nil {
		if !intent.existingBootstrap() {
			return errors.New("staging cannot mutate a live issuer before the loaded-trust barrier")
		}
		object, err := w.store.dynamic.Resource(rotationIssuerResource).Namespace(w.store.namespace).
			Get(ctx, intent.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		pin, err := bootstrapResourcePin(object)
		if err != nil {
			return err
		}
		if pin.UID != intent.Before.UID || pin.SpecSHA256 != intent.Before.SpecSHA256 ||
			!rotationOwned(object, w.store.namespace, w.store.release) || len(object.GetOwnerReferences()) != 0 ||
			intent.Observed == nil && pin.ResourceVersion != intent.Before.ResourceVersion {
			return errors.New("existing bootstrap issuer differs from its owned before snapshot")
		}
		if intent.Observed != nil {
			return nil
		}
		next, err := cloneCertificateLifecycle(w.state)
		if err != nil {
			return err
		}
		next.Execution.Resources[index].Observed = &pin
		return w.save(ctx, next)
	}
	encoded, err := json.Marshal(map[string]any{
		"apiVersion": "cert-manager.io/v1", "kind": intent.Kind,
		"metadata": map[string]any{"name": intent.Name, "namespace": w.store.namespace,
			"labels": map[string]any{helmManagedByLabel: helmManagedByValue},
			"annotations": map[string]any{helmReleaseNameAnnotation: w.store.release,
				helmReleaseNamespaceAnnotation: w.store.namespace, certificateExecutionOwner: string(w.journal.UID),
				"helm.sh/resource-policy": "keep"}},
		"spec": intent.Spec,
	})
	if err != nil {
		return err
	}
	desired := &unstructured.Unstructured{}
	if err := desired.UnmarshalJSON(encoded); err != nil {
		return err
	}
	client := w.store.dynamic.Resource(certificateIntentResource(intent.Kind)).Namespace(w.store.namespace)
	object, err := client.Get(ctx, intent.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if intent.Observed != nil {
			return errors.New("observed staged resource disappeared")
		}
		if err := w.check(ctx); err != nil {
			return err
		}
		object, err = client.Create(ctx, desired, metav1.CreateOptions{FieldManager: FieldManager})
	}
	if err != nil {
		return err
	}
	pin, err := bootstrapResourcePin(object)
	if err != nil {
		return err
	}
	if !rotationOwned(object, w.store.namespace, w.store.release) || len(object.GetOwnerReferences()) != 0 ||
		object.GetAnnotations()[certificateExecutionOwner] != string(w.journal.UID) ||
		object.GetAnnotations()["helm.sh/resource-policy"] != "keep" ||
		!apiequality.Semantic.DeepDerivative(desired.Object["spec"], object.Object["spec"]) {
		return errors.New("staged resource has foreign ownership or changed configuration")
	}
	if intent.Observed != nil {
		if intent.Observed.UID != pin.UID || intent.Observed.SpecSHA256 != pin.SpecSHA256 {
			return errors.New("staged resource was replaced or reconfigured")
		}
		return nil
	}
	next, err := cloneCertificateLifecycle(w.state)
	if err != nil {
		return err
	}
	next.Execution.Resources[index].Observed = &pin
	return w.save(ctx, next)
}

func cloneCertificateLifecycle(state certificateLifecycle) (certificateLifecycle, error) {
	data, err := json.Marshal(state)
	if err != nil {
		return certificateLifecycle{}, err
	}
	var result certificateLifecycle
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, err
	}
	return result, nil
}

func (w *certificateExecutionWriter) publishSource(ctx context.Context, name, revision string) error {
	if w.journal == nil || !w.state.Planes[name].Affected {
		return errors.New("publication requires a recorded affected plane")
	}
	if err := w.check(ctx); err != nil {
		return err
	}
	if w.state.Phase == rotationRetire && w.state.Execution.PendingSource == nil {
		if err := w.checkRetirementEvidence(ctx); err != nil {
			return err
		}
	}
	plane := w.state.Planes[name]
	bundle := bytes.Clone(plane.NewCA)
	if w.state.Phase == certificatePrepare {
		bundle = append(bytes.Clone(plane.OldCA), bundle...)
	}
	if err := exactCertificateSet(bundle, bundle, w.store.now()); err != nil {
		return err
	}
	if plane.Source.Revision == revision && plane.Source.JournalUID == w.journal.UID &&
		bytes.Equal(plane.Source.Bundle, bundle) && w.state.Execution.PendingSource == nil {
		return nil
	}
	if w.state.Execution.PendingSource == nil {
		next, err := cloneCertificateLifecycle(w.state)
		if err != nil {
			return err
		}
		next.Execution.PendingSource = &certificateSourceIntent{Plane: name, Before: plane.Source, Bundle: bundle, Revision: revision}
		if err := w.save(ctx, next); err != nil {
			return err
		}
	}
	pending := w.state.Execution.PendingSource
	if pending.Plane != name || pending.Revision != revision || !bytes.Equal(pending.Bundle, bundle) {
		return errors.New("finish the recorded public-source write before another publication")
	}
	object, actual, err := w.readSource(ctx, name)
	if err != nil {
		return err
	}
	if err := w.check(ctx); err != nil {
		return err
	}
	annotations := object.GetAnnotations()
	if !bytes.Equal(actual, pending.Bundle) || annotations[rotationJournalUID] != string(w.journal.UID) ||
		annotations[rotationRevision] != pending.Revision || annotations["radius.io/certificate-mode"] != w.state.Mode {
		if object.GetUID() != pending.Before.UID || object.GetResourceVersion() != pending.Before.ResourceVersion {
			return errors.New("public source changed before conditional publication")
		}
		if w.state.Phase == rotationRetire {
			if err := w.checkRetirementEvidence(ctx); err != nil {
				return err
			}
			if err := w.check(ctx); err != nil {
				return err
			}
		}
		annotations = maps.Clone(annotations)
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[rotationJournalUID], annotations[rotationRevision] = string(w.journal.UID), pending.Revision
		annotations["radius.io/certificate-mode"] = w.state.Mode
		switch source := object.(type) {
		case *corev1.ConfigMap:
			copy := source.DeepCopy()
			copy.Annotations, copy.Data = annotations, map[string]string{caCertificateKey: string(bundle)}
			object, err = w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Update(ctx, copy, metav1.UpdateOptions{FieldManager: FieldManager})
		case *corev1.Secret:
			copy := source.DeepCopy()
			copy.Annotations, copy.Data = annotations, map[string][]byte{caCertificateKey: bundle}
			object, err = w.store.kube.CoreV1().Secrets(w.store.namespace).Update(ctx, copy, metav1.UpdateOptions{FieldManager: FieldManager})
		default:
			return errors.New("unsupported public-source object")
		}
		if err != nil {
			return err
		}
	}
	if object.GetUID() != pending.Before.UID || object.GetResourceVersion() == "" {
		return errors.New("public-source write returned an invalid identity")
	}
	next, err := cloneCertificateLifecycle(w.state)
	if err != nil {
		return err
	}
	updated := next.Planes[name]
	updated.Source.Bundle = bytes.Clone(bundle)
	updated.Source.ResourceVersion = object.GetResourceVersion()
	updated.Source.SHA256 = fmt.Sprintf("%x", sha256.Sum256(bundle))
	updated.Source.JournalUID, updated.Source.Revision = w.journal.UID, revision
	next.Planes[name], next.Execution.PendingSource = updated, nil
	return w.save(ctx, next)
}

func (w *certificateExecutionWriter) checkRetirementEvidence(ctx context.Context) error {
	if w.observeLoaded == nil {
		return errCertificateObserverUnavailable
	}
	if w.observeServing == nil {
		return errors.New("retirement requires separate exact serving-pod evidence")
	}
	if err := w.observeLoaded(ctx, w.state); err != nil {
		return err
	}
	return w.observeServing(ctx, w.state)
}

func (w *certificateExecutionWriter) checkAggregationTrust(ctx context.Context) error {
	plane, found := w.state.Planes["aggregation"]
	if !found || !plane.Affected {
		return nil
	}

	api, err := w.store.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if err := w.store.checkLifecycleAPIService(api); err != nil {
		return err
	}
	if api.GetUID() != w.state.APIServiceUID {
		return errors.New("APIService was replaced during trust convergence")
	}
	encoded, _, err := unstructured.NestedString(api.Object, "spec", "caBundle")
	if err != nil {
		return err
	}
	bundle, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	if err := exactCertificateSet(bundle, plane.Source.Bundle, w.store.now()); err != nil {
		return fmt.Errorf("APIService has not converged to the exact selected public trust: %w", err)
	}
	return ctx.Err()
}

// The journal/source already declare the exact desired public set. CAS repairs
// a merging injector's retained old root without retargeting the source or
// changing any shared controller. A later hostile writer is not fenced forever.
func (w *certificateExecutionWriter) reconcileAggregationTrust(ctx context.Context) error {
	plane, found := w.state.Planes["aggregation"]
	if !found || !plane.Affected {
		return nil
	}
	if w.state.Phase != rotationPublish && w.state.Phase != rotationRetire {
		return errors.New("APIService trust writes require explicit publish or retire intent")
	}
	if w.state.Execution.CAInjectorSHA256 == "" {
		return errors.New("APIService trust writes require observed cainjector configuration and ownership")
	}
	if err := w.check(ctx); err != nil {
		return err
	}
	api, err := w.store.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if err := w.store.checkLifecycleAPIService(api); err != nil {
		return err
	}
	if api.GetUID() != w.state.APIServiceUID || api.GetResourceVersion() == "" {
		return errors.New("APIService changed before conditional trust convergence")
	}
	actual, err := apiServiceBundle(api)
	if err != nil {
		return err
	}
	if exactCertificateSet(actual, plane.Source.Bundle, w.store.now()) == nil {
		return nil
	}
	before := plane.OldCA
	if w.state.Phase == rotationRetire {
		before = append(bytes.Clone(plane.OldCA), plane.NewCA...)
		if w.observeServing == nil {
			return errors.New("aggregation retirement requires exact serving-pod evidence")
		}
		if err := w.observeServing(ctx, w.state); err != nil {
			return err
		}
	}
	if err := exactCertificateSet(actual, before, w.store.now()); err != nil {
		return errors.New("APIService contains unplanned trust; do not overwrite unknown anchors")
	}
	if err := w.check(ctx); err != nil {
		return err
	}
	if err := unstructured.SetNestedField(api.Object, base64.StdEncoding.EncodeToString(plane.Source.Bundle), "spec", "caBundle"); err != nil {
		return err
	}
	_, err = w.store.dynamic.Resource(apiServiceResource).Update(ctx, api, metav1.UpdateOptions{FieldManager: FieldManager})
	return err
}
