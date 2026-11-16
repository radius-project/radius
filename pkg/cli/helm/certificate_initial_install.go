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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
	helm "helm.sh/helm/v4/pkg/action"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	initialCertificateJournal = "radius-certificate-bootstrap"
	initialCertificateIntent  = "radius.io/certificate-bootstrap"
)

type initialCertificateAnchor struct {
	CA     []byte                `json:"ca"`
	Signer *certificateObjectPin `json:"signer,omitempty"`
}

// This journal describes a new installation, never an old/new overlap or loaded
// consumer acknowledgement. Unobserved resources have no invented UID or revision.
type initialCertificateState struct {
	Version      int                                   `json:"version"`
	ID           string                                `json:"id"`
	Namespace    string                                `json:"namespace"`
	NamespaceUID types.UID                             `json:"namespaceUID"`
	Release      string                                `json:"release"`
	Mode         string                                `json:"mode"`
	Provider     string                                `json:"provider"`
	Phase        string                                `json:"phase"`
	ChartSHA256  string                                `json:"chartSHA256"`
	IntentSHA256 string                                `json:"intentSHA256"`
	ValuesSHA256 string                                `json:"valuesSHA256"`
	Roots        map[string]aggregationBootstrapPin    `json:"roots"`
	Anchors      map[string]initialCertificateAnchor   `json:"anchors"`
	Sources      map[string]certificateTrustProjection `json:"sources"`
	Issuers      map[string]certificateIssuerSnapshot  `json:"issuers"`
}

type initialCertificateInstaller struct {
	impl        *Impl
	store       *rotationStore
	options     ClusterOptions
	selected    *chart.Chart
	helm        *helm.Configuration
	values      map[string]any
	kubeContext string
	importPlan  *certificatePlan
	target      *certificateTarget
	state       initialCertificateState
	journal     *corev1.ConfigMap
	operation   *identityOperation
	// Only local controller fixtures replace these boundaries. Production uses
	// the selected Helm chart and the ordinary exact-pod/APIService checks.
	applyFixture  func(context.Context) error
	verifyFixture func(context.Context) error
}

func initialCertificateValuesDigest(values map[string]any) (string, error) {
	copy := copyValues(values)
	// The CSI dependency planner supplies this list after read-only preparation.
	// The complete post-dependency values are pinned separately before CA writes.
	unstructured.RemoveNestedField(copy, "global", "rbac", "certManager", "requesters")
	return bootstrapDigest(copy)
}

func readInitialCertificateJournal(ctx context.Context, store *rotationStore) (*corev1.ConfigMap, initialCertificateState, error) {
	var state initialCertificateState
	journal, err := store.kube.CoreV1().ConfigMaps(store.namespace).Get(ctx, initialCertificateJournal, metav1.GetOptions{})
	if err != nil {
		return nil, state, err
	}
	if journal.UID == "" || journal.ResourceVersion == "" || journal.DeletionTimestamp != nil ||
		len(journal.OwnerReferences) != 0 || journal.Labels[rotationOwner] != store.release ||
		journal.Annotations[helmReleaseNameAnnotation] != "" || journal.Annotations[helmReleaseNamespaceAnnotation] != "" ||
		journal.Labels[helmManagedByLabel] == helmManagedByValue || len(journal.Data) != 1 || len(journal.BinaryData) != 0 {
		return nil, state, errors.New("initial certificate journal has foreign ownership or incomplete identity")
	}
	raw := journal.Data[rotationJournalKey]
	state, err = decodeInitialCertificateState(raw, store, true)
	return journal, state, err
}

func decodeInitialCertificateState(raw string, store *rotationStore, bound bool) (initialCertificateState, error) {
	var state initialCertificateState
	if len(raw) == 0 || len(raw) > maxRotationBytes {
		return state, errors.New("initial certificate journal exceeds its bounds")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return state, errors.New("invalid initial certificate journal")
	}
	if err := checkRotationJSON(decoder, first, 0); err != nil {
		return state, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return state, errors.New("trailing initial certificate journal data")
	}
	decoder = json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return state, err
	}
	id, err := uuid.Parse(state.ID)
	if err != nil || id == uuid.Nil || id.String() != state.ID || state.Version != 1 ||
		state.Namespace != store.namespace || bound && state.NamespaceUID == "" || state.Release != store.release ||
		(state.Mode != "selfSigned" && state.Mode != "caSecret" && state.Mode != "issuerRef") ||
		(state.Provider != "radius" && state.Provider != "external") ||
		(state.Phase != "preparing" && state.Phase != "published" && state.Phase != "complete") ||
		!validCertificateHash(state.ChartSHA256) || !validCertificateHash(state.IntentSHA256) ||
		!validCertificateHash(state.ValuesSHA256) || state.Roots == nil || state.Anchors == nil || state.Sources == nil || state.Issuers == nil {
		return state, errors.New("invalid initial certificate transaction identity or configuration")
	}
	if !bound && (state.NamespaceUID != "" || state.Phase != "preparing" ||
		len(state.Roots)+len(state.Anchors)+len(state.Sources)+len(state.Issuers) != 0) {
		return state, errors.New("namespace intent must not contain unobserved bootstrap material")
	}
	for key, pin := range state.Roots {
		_, name, err := initialRootResource(key)
		if err != nil || state.Mode != "selfSigned" || state.Provider == "external" && name == "radius-ca" ||
			pin.UID == "" || pin.ResourceVersion == "" || !validCertificateHash(pin.SpecSHA256) {
			return state, errors.New("invalid bootstrap root observation")
		}
	}
	validPlane := func(plane string) bool {
		return plane == "aggregation" || plane == "internal" && state.Provider == "radius"
	}
	for plane, anchor := range state.Anchors {
		certificates, err := parseCertificateBundlePEM(anchor.CA)
		if !validPlane(plane) || err != nil || len(certificates) != 1 || !certificates[0].IsCA {
			return state, errors.New("invalid bootstrap anchor observation")
		}
		if state.Mode == "issuerRef" {
			if anchor.Signer != nil {
				return state, errors.New("external bootstrap must not record a local signer")
			}
			if _, found := state.Issuers[plane]; !found {
				return state, errors.New("external bootstrap anchor lacks its issuer observation")
			}
		} else if anchor.Signer == nil || anchor.Signer.Name == "" || anchor.Signer.UID == "" ||
			anchor.Signer.ResourceVersion == "" || anchor.Signer.SHA256 != fmt.Sprintf("%x", sha256.Sum256(certificates[0].Raw)) {
			return state, errors.New("bootstrap anchor lacks its matching signer observation")
		}
	}
	for plane, issuer := range state.Issuers {
		if !validPlane(plane) || state.Mode != "issuerRef" {
			return state, errors.New("invalid bootstrap external issuer plane")
		}
		if err := issuer.validate(store.namespace); err != nil {
			return state, err
		}
	}
	for plane, source := range state.Sources {
		kind, name := "Secret", ucpAPIServerTrustSecretName
		if plane == "internal" {
			kind, name = "ConfigMap", "radius-mtls-ca"
		}
		if !validPlane(plane) || source.Kind != kind || source.Name != name ||
			source.UID == "" || source.ResourceVersion == "" || len(source.Bundle) == 0 ||
			source.SHA256 != fmt.Sprintf("%x", sha256.Sum256(source.Bundle)) ||
			!bytes.Equal(source.Bundle, state.Anchors[plane].CA) {
			return state, errors.New("invalid bootstrap public-source observation")
		}
	}
	if state.Phase != "preparing" {
		count := 1
		if state.Provider == "radius" {
			count = 2
		}
		if len(state.Anchors) != count || len(state.Sources) != count ||
			state.Mode == "selfSigned" && len(state.Roots) != count+1 {
			return state, errors.New("bootstrap publication is incomplete")
		}
	}
	return state, nil
}

func (b *initialCertificateInstaller) check(ctx context.Context) error {
	if err := b.operation.checkProviderJournal(ctx, b.store.release, b.impl.validateCompletedProviderJournal); err != nil {
		return err
	}
	return b.checkJournal(ctx)
}

func (b *initialCertificateInstaller) checkJournal(ctx context.Context) error {
	namespace, err := b.store.kube.CoreV1().Namespaces().Get(ctx, b.store.namespace, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if namespace.UID != b.state.NamespaceUID || namespace.DeletionTimestamp != nil {
		return errors.New("bootstrap namespace was replaced or is terminating")
	}
	if _, err := b.store.kube.CoreV1().ConfigMaps(b.store.namespace).Get(ctx, rotationJournalName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		if err != nil {
			return err
		}
		return errors.New("CA rotation appeared during fresh installation")
	}
	if b.journal != nil {
		current, _, err := readInitialCertificateJournal(ctx, b.store)
		if err != nil {
			return err
		}
		if current.UID != b.journal.UID || current.ResourceVersion != b.journal.ResourceVersion ||
			!reflect.DeepEqual(current.Data, b.journal.Data) {
			return errors.New("initial certificate journal changed during execution")
		}
	}
	return ctx.Err()
}

func (b *initialCertificateInstaller) save(ctx context.Context) error {
	if err := b.check(ctx); err != nil {
		return err
	}
	data, err := json.Marshal(b.state)
	if err != nil || len(data) > maxRotationBytes {
		return errors.New("initial certificate journal exceeds its size limit")
	}
	updated := b.journal.DeepCopy()
	updated.Data = map[string]string{rotationJournalKey: string(data)}
	persisted, err := b.store.kube.CoreV1().ConfigMaps(b.store.namespace).Update(ctx, updated, metav1.UpdateOptions{FieldManager: FieldManager})
	if err == nil {
		b.journal = persisted
	}
	return err
}

func (b *initialCertificateInstaller) run(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, effectiveTimeout(b.options.Radius.Timeout))
	defer cancel()
	values, err := valuesutil.CoalesceValues(b.selected, b.values)
	if err != nil {
		return err
	}
	b.values = values
	valuesHash, err := bootstrapDigest(values)
	if err != nil {
		return err
	}
	if b.journal != nil && b.state.ValuesSHA256 != valuesHash {
		return errors.New("bootstrap retry changed the effective dependency/requester configuration")
	}
	b.state.ValuesSHA256 = valuesHash
	namespace, err := b.store.kube.CoreV1().Namespaces().Get(ctx, b.store.namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if err := ctx.Err(); err != nil {
			return err
		}
		intent, err := json.Marshal(b.state)
		if err != nil {
			return err
		}
		namespace, err = b.store.kube.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: b.store.namespace, Annotations: map[string]string{initialCertificateIntent: string(intent)},
		}}, metav1.CreateOptions{FieldManager: FieldManager})
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if namespace.DeletionTimestamp != nil {
		return errors.New("bootstrap namespace is terminating")
	}
	if b.journal == nil && namespace.Annotations[initialCertificateIntent] != "" {
		intent, err := decodeInitialCertificateState(namespace.Annotations[initialCertificateIntent], b.store, false)
		if err != nil {
			return err
		}
		if intent.ChartSHA256 != b.state.ChartSHA256 || intent.IntentSHA256 != b.state.IntentSHA256 ||
			intent.ValuesSHA256 != b.state.ValuesSHA256 || intent.Mode != b.state.Mode || intent.Provider != b.state.Provider {
			return errors.New("namespace retains a different initial certificate intent")
		}
		b.state.ID = intent.ID
	}
	if b.state.NamespaceUID != "" && b.state.NamespaceUID != namespace.UID {
		return errors.New("bootstrap belongs to another namespace instance")
	}
	b.state.NamespaceUID = namespace.UID
	return withIdentityOperation(ctx, b.store.kube, b.store.namespace, func(operation *identityOperation) error {
		b.operation = operation
		if b.journal == nil {
			if err := b.checkFresh(ctx); err != nil {
				return err
			}
			if err := b.check(ctx); err != nil {
				return err
			}
			data, err := json.Marshal(b.state)
			if err != nil {
				return err
			}
			b.journal, err = b.store.kube.CoreV1().ConfigMaps(b.store.namespace).Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: initialCertificateJournal, Namespace: b.store.namespace,
					Labels: map[string]string{rotationOwner: b.store.release}},
				Data: map[string]string{rotationJournalKey: string(data)},
			}, metav1.CreateOptions{FieldManager: FieldManager})
			if err != nil {
				return err
			}
		}
		if err := b.checkMaterial(ctx); err != nil {
			return err
		}
		if b.state.Phase == "preparing" {
			if err := b.prepareAnchors(ctx); err != nil {
				return err
			}
			if err := b.publish(ctx); err != nil {
				return err
			}
			b.state.Phase = "published"
			if err := b.save(ctx); err != nil {
				return err
			}
		}
		if b.state.Phase != "published" {
			return errors.New("completed bootstrap is not a new installation")
		}
		if err := b.checkMaterial(ctx); err != nil {
			return err
		}
		if b.applyFixture != nil {
			err = b.applyFixture(ctx)
		} else {
			err = b.applyChart(ctx)
		}
		if err != nil {
			return fmt.Errorf("initial certificate installation is unfinished; retry identical install settings: %w", err)
		}
		if b.verifyFixture != nil {
			err = b.verifyFixture(ctx)
		} else {
			err = b.impl.waitForInstalledUCPAPIService(ctx, b.options, b.kubeContext, b.options.RadiusReleaseValues)
		}
		if err != nil {
			return err
		}
		if err := b.checkMaterial(ctx); err != nil {
			return err
		}
		b.state.Phase = "complete"
		return b.save(ctx)
	})
}

func (b *initialCertificateInstaller) checkFresh(ctx context.Context) error {
	releases, err := b.store.kube.CoreV1().Secrets(b.store.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "owner=helm,name=" + b.store.release,
	})
	if err != nil {
		return err
	}
	if len(releases.Items) != 0 {
		return errors.New("fresh certificate bootstrap cannot adopt an existing Helm release")
	}
	for _, entry := range []struct {
		resource schema.GroupVersionResource
		names    []string
	}{
		{rotationIssuerResource, []string{"radius-selfsigned-bootstrap", "radius-ca-issuer", "radius-apiserver-issuer"}},
		{rotationCertificateResource, []string{"radius-ca", "radius-apiserver-ca"}},
	} {
		for _, name := range entry.names {
			if _, err := b.store.dynamic.Resource(entry.resource).Namespace(b.store.namespace).Get(ctx, name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
				if err != nil {
					return err
				}
				return fmt.Errorf("fresh bootstrap cannot adopt existing %s %s", entry.resource.Resource, name)
			}
		}
	}
	if _, err := b.store.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		if err != nil {
			return err
		}
		return errors.New("fresh certificate bootstrap cannot adopt an existing APIService")
	}
	for _, name := range []string{ucpAPIServerTrustSecretName, "radius-apiserver-ca", "radius-ca"} {
		if b.state.Mode != "selfSigned" && name != ucpAPIServerTrustSecretName {
			continue
		}
		if _, err := b.store.kube.CoreV1().Secrets(b.store.namespace).Get(ctx, name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			if err != nil {
				return err
			}
			return fmt.Errorf("fresh bootstrap cannot adopt existing Secret %s", name)
		}
	}
	if _, err := b.store.kube.CoreV1().ConfigMaps(b.store.namespace).Get(ctx, "radius-mtls-ca", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		if err != nil {
			return err
		}
		return errors.New("fresh bootstrap cannot adopt existing internal public trust")
	}
	return ctx.Err()
}

func (b *initialCertificateInstaller) checkMaterial(ctx context.Context) error {
	if err := b.check(ctx); err != nil {
		return err
	}
	return b.checkObservedMaterial(ctx)
}

func (b *initialCertificateInstaller) checkPrepared(ctx context.Context) error {
	if err := b.checkJournal(ctx); err != nil {
		return err
	}
	if err := b.checkObservedMaterial(ctx); err != nil {
		return err
	}
	return b.checkJournal(ctx)
}

func (b *initialCertificateInstaller) checkObservedMaterial(ctx context.Context) error {
	for key, pin := range b.state.Roots {
		resource, name, err := initialRootResource(key)
		if err != nil {
			return err
		}
		current, err := b.store.dynamic.Resource(resource).Namespace(b.store.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		observed, err := bootstrapResourcePin(current)
		if err != nil {
			return err
		}
		if observed.UID != pin.UID || observed.SpecSHA256 != pin.SpecSHA256 ||
			!rotationOwned(current, b.store.namespace, b.store.release) ||
			current.GetAnnotations()[initialCertificateIntent] != string(b.journal.UID) {
			return errors.New("bootstrap root resource identity, configuration or ownership changed")
		}
	}
	for _, pin := range b.state.Issuers {
		if err := b.store.checkCertificateIssuerSnapshot(ctx, pin, rotationPublish); err != nil {
			return err
		}
	}
	for _, anchor := range b.state.Anchors {
		if _, err := validatePublicCA(anchor.CA, b.store.now()); err != nil {
			return err
		}
		if err := b.store.checkCertificateSignerSnapshot(ctx, anchor.Signer, rotationPublish); err != nil {
			return err
		}
	}
	for plane, pin := range b.state.Sources {
		var source metav1.Object
		var bundle []byte
		if plane == "aggregation" {
			object, err := b.store.kube.CoreV1().Secrets(b.store.namespace).Get(ctx, pin.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if err := publicAggregationSource(object, b.store.namespace); err != nil {
				return err
			}
			source, bundle = object, object.Data[caCertificateKey]
		} else {
			object, err := b.store.kube.CoreV1().ConfigMaps(b.store.namespace).Get(ctx, pin.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if len(object.Data) != 1 || len(object.BinaryData) != 0 {
				return errors.New("bootstrap internal source is not public-only")
			}
			source, bundle = object, []byte(object.Data[caCertificateKey])
		}
		if source.GetUID() != pin.UID || source.GetDeletionTimestamp() != nil ||
			len(source.GetOwnerReferences()) != 0 || source.GetLabels()[helmManagedByLabel] != helmManagedByValue ||
			source.GetAnnotations()[helmReleaseNameAnnotation] != b.store.release ||
			source.GetAnnotations()[helmReleaseNamespaceAnnotation] != b.store.namespace ||
			source.GetAnnotations()[initialCertificateIntent] != string(b.journal.UID) ||
			source.GetAnnotations()["radius.io/certificate-mode"] != b.state.Mode ||
			source.GetAnnotations()["helm.sh/resource-policy"] != "keep" ||
			!bytes.Equal(bundle, pin.Bundle) || !bytes.Equal(bundle, b.state.Anchors[plane].CA) {
			return errors.New("bootstrap public trust identity, ownership or exact bytes changed")
		}
	}
	return ctx.Err()
}

func (b *initialCertificateInstaller) publish(ctx context.Context) error {
	for _, plane := range b.planeNames() {
		if _, found := b.state.Sources[plane]; found {
			continue
		}
		if err := b.checkMaterial(ctx); err != nil {
			return err
		}
		ca := b.state.Anchors[plane].CA
		if _, err := validatePublicCA(ca, b.store.now()); err != nil {
			return err
		}
		metadata := metav1.ObjectMeta{Namespace: b.store.namespace, Labels: map[string]string{helmManagedByLabel: helmManagedByValue},
			Annotations: map[string]string{helmReleaseNameAnnotation: b.store.release, helmReleaseNamespaceAnnotation: b.store.namespace,
				initialCertificateIntent: string(b.journal.UID), "radius.io/certificate-mode": b.state.Mode, "helm.sh/resource-policy": "keep"}}
		var observed metav1.Object
		kind := "ConfigMap"
		if plane == "aggregation" {
			kind, metadata.Name = "Secret", ucpAPIServerTrustSecretName
			metadata.Annotations[allowDirectInjectionAnnotation] = "true"
			client := b.store.kube.CoreV1().Secrets(b.store.namespace)
			object, err := client.Get(ctx, metadata.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				if err := b.check(ctx); err != nil {
					return err
				}
				object, err = client.Create(ctx, &corev1.Secret{ObjectMeta: metadata, Type: corev1.SecretTypeOpaque,
					Data: map[string][]byte{caCertificateKey: ca}}, metav1.CreateOptions{FieldManager: FieldManager})
			}
			if err != nil {
				return err
			}
			if err := publicAggregationSource(object, b.store.namespace); err != nil {
				return err
			}
			if !bytes.Equal(object.Data[caCertificateKey], ca) {
				return errors.New("bootstrap retry found different aggregation trust")
			}
			observed = object
		} else {
			metadata.Name = "radius-mtls-ca"
			client := b.store.kube.CoreV1().ConfigMaps(b.store.namespace)
			object, err := client.Get(ctx, metadata.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				if err := b.check(ctx); err != nil {
					return err
				}
				object, err = client.Create(ctx, &corev1.ConfigMap{ObjectMeta: metadata,
					Data: map[string]string{caCertificateKey: string(ca)}}, metav1.CreateOptions{FieldManager: FieldManager})
			}
			if err != nil {
				return err
			}
			if len(object.Data) != 1 || len(object.BinaryData) != 0 || object.Data[caCertificateKey] != string(ca) {
				return errors.New("bootstrap retry found different internal trust")
			}
			observed = object
		}
		if observed.GetAnnotations()[initialCertificateIntent] != string(b.journal.UID) ||
			observed.GetUID() == "" || observed.GetResourceVersion() == "" {
			return errors.New("bootstrap cannot adopt a public source without its creation intent")
		}
		b.state.Sources = maps.Clone(b.state.Sources)
		b.state.Sources[plane] = certificateTrustProjection{Kind: kind, Bundle: bytes.Clone(ca),
			certificateObjectPin: certificateObjectPin{Name: metadata.Name, UID: observed.GetUID(),
				ResourceVersion: observed.GetResourceVersion(), SHA256: fmt.Sprintf("%x", sha256.Sum256(ca))}}
		if err := b.checkMaterial(ctx); err != nil {
			return err
		}
		if err := b.save(ctx); err != nil {
			return err
		}
	}
	return b.checkMaterial(ctx)
}

func (b *initialCertificateInstaller) planeNames() []string {
	if b.state.Provider == "external" {
		return []string{"aggregation"}
	}
	return []string{"internal", "aggregation"}
}

func (b *initialCertificateInstaller) prepareAnchors(ctx context.Context) error {
	if b.state.Mode == "selfSigned" {
		roots, err := b.initialRoots(ctx)
		if err != nil {
			return err
		}
		for _, root := range roots {
			if err := b.ensureRoot(ctx, root); err != nil {
				return err
			}
		}
	} else if b.importPlan != nil {
		b.importPlan.requiresLease = true
		b.importPlan.beforeWrite = b.check
		if err := b.impl.applyCertificatePlan(ctx, b.importPlan); err != nil {
			return err
		}
	}
	for _, plane := range b.planeNames() {
		if _, found := b.state.Anchors[plane]; found {
			continue
		}
		anchor := initialCertificateAnchor{}
		if b.state.Mode == "issuerRef" {
			selection := b.target.Planes[plane]
			pin, err := b.store.snapshotCertificateIssuer(ctx, selection.Issuer)
			if err != nil {
				return err
			}
			b.state.Issuers[plane], anchor.CA = pin, bytes.Clone(selection.CA)
		} else {
			name := b.target.SigningSecret
			if b.state.Mode == "selfSigned" {
				name = "radius-ca"
				if plane == "aggregation" {
					name = "radius-apiserver-ca"
				}
			}
			err := wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, effectiveTimeout(b.options.Radius.Timeout), true, func(ctx context.Context) (bool, error) {
				if err := b.checkMaterial(ctx); err != nil {
					return false, err
				}
				secret, err := b.store.kube.CoreV1().Secrets(b.store.namespace).Get(ctx, name, metav1.GetOptions{})
				if apierrors.IsNotFound(err) && b.state.Mode == "selfSigned" {
					return false, nil
				}
				if err != nil {
					return false, err
				}
				certificate, err := validateSigningCA(secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey], b.store.now())
				if err != nil {
					return false, err
				}
				if secret.Type != corev1.SecretTypeTLS || secret.UID == "" || secret.ResourceVersion == "" || secret.DeletionTimestamp != nil {
					return false, errors.New("bootstrap signer has no live TLS Secret identity")
				}
				if b.state.Mode == "caSecret" && b.target.SigningUID != "" &&
					(secret.UID != b.target.SigningUID || secret.ResourceVersion != b.target.SigningVersion) {
					return false, errors.New("bootstrap signing Secret changed since target preparation")
				}
				ca := b.target.Planes[plane].CA
				if b.state.Mode == "selfSigned" {
					ca = secret.Data[caCertificateKey]
					if secret.Annotations["cert-manager.io/certificate-name"] != name ||
						certificate.CheckSignatureFrom(certificate) != nil {
						return false, errors.New("bootstrap root is not the selected selfSigned Certificate's Secret")
					}
					ready, err := b.rootIssued(ctx, name, secret)
					if err != nil || !ready {
						return false, err
					}
				}
				if !sameCACertificate(string(ca), certificate) {
					return false, errors.New("bootstrap signing Secret differs from the selected public anchor")
				}
				chain, err := parseCertificateBundlePEM(secret.Data[corev1.TLSCertKey])
				if err != nil {
					return false, err
				}
				if err := validateCAProfiles(chain, b.target, b.store.namespace, b.store.now()); err != nil {
					return false, err
				}
				anchor = initialCertificateAnchor{CA: bytes.Clone(ca), Signer: &certificateObjectPin{
					Name: name, UID: secret.UID, ResourceVersion: secret.ResourceVersion,
					SHA256: fmt.Sprintf("%x", sha256.Sum256(certificate.Raw))}}
				return true, nil
			})
			if err != nil {
				return err
			}
		}
		if _, _, err := mtls.TrustBundleDigest(anchor.CA, b.store.now()); err != nil {
			return err
		}
		b.state.Anchors[plane] = anchor
		if err := b.save(ctx); err != nil {
			return err
		}
	}
	return b.checkMaterial(ctx)
}
