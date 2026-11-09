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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	rotationJournalName = "radius-ca-rotation"
	rotationJournalKey  = "state.json"
	rotationOwner       = "radius.io/trust-owner"
	rotationRevision    = "radius.io/trust-revision"
	rotationJournalUID  = "radius.io/trust-journal-uid"
	radiusAPIService    = "v1alpha3.api.ucp.dev"
	maxRotationBytes    = 64 * 1024
)

var apiServiceResource = schema.GroupVersionResource{
	Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices",
}

type rotationPhase string

const (
	rotationPublish  rotationPhase = "publish"
	rotationAdopt    rotationPhase = "adopt"
	rotationIssue    rotationPhase = "issue"
	rotationReady    rotationPhase = "ready"
	rotationRetire   rotationPhase = "retire"
	rotationComplete rotationPhase = "complete"
)

// rotationState records public intent, not proof that the current phase has finished.
// The orchestration must establish each phase's consumer/serving barriers before advancing.
type rotationState struct {
	SchemaVersion    int           `json:"schemaVersion"`
	ID               string        `json:"id"`
	Namespace        string        `json:"namespace"`
	Release          string        `json:"release"`
	APIServiceUID    types.UID     `json:"apiServiceUID"`
	Phase            rotationPhase `json:"phase"`
	CreatedAt        time.Time     `json:"createdAt"`
	OldCA            []byte        `json:"oldCA"`
	NewCA            []byte        `json:"newCA"`
	PreviousRevision string        `json:"previousRevision,omitempty"`
	Plan             *rotationPlan `json:"plan,omitempty"`
	Recovery         bool          `json:"recovery,omitempty"`
	PriorRevisions   []string      `json:"priorRevisions,omitempty"`
}

var rotationPhases = []rotationPhase{rotationPublish, rotationAdopt, rotationIssue, rotationReady, rotationRetire, rotationComplete}

// rotationPlan contains only public material and resource identities. Signing keys
// remain in the existing operator-owned or create-only imported Secret.
type rotationPlan struct {
	Scope                      rotationScope                `json:"scope,omitempty"`
	Aggregation                *rotationAggregationWorkload `json:"aggregation,omitempty"`
	ExternalIssuer             *rotationExternalIssuer      `json:"externalIssuer,omitempty"`
	TargetMode                 string                       `json:"targetMode"`
	TargetTrustMap             string                       `json:"targetTrustMap"`
	TargetTrustSHA256          string                       `json:"targetTrustSHA256"`
	OldComponentCA             []byte                       `json:"oldComponentCA,omitempty"`
	SigningSecret              string                       `json:"signingSecret,omitempty"`
	SigningUID                 types.UID                    `json:"signingUID,omitempty"`
	Issuer                     string                       `json:"issuer"`
	ServingSecret              string                       `json:"servingSecret"`
	Consumers                  []rotationWorkload           `json:"consumers,omitempty"`
	ServiceUID                 types.UID                    `json:"serviceUID"`
	ServiceSelector            map[string]string            `json:"serviceSelector"`
	UCPMinReadySeconds         int32                        `json:"ucpMinReadySeconds"`
	UCPProgressDeadlineSeconds int32                        `json:"ucpProgressDeadlineSeconds"`
}

type rotationWorkload struct {
	Name           string    `json:"name"`
	UID            types.UID `json:"uid"`
	CertificateUID types.UID `json:"certificateUID"`
	OldIssuer      string    `json:"oldIssuer"`
	OldIssuerGroup string    `json:"oldIssuerGroup,omitempty"`
	OldIssuerKind  string    `json:"oldIssuerKind,omitempty"`
	ProofPort      uint16    `json:"proofPort"`
}

func (s rotationState) validate(now time.Time) error {
	id, err := uuid.Parse(s.ID)
	if err != nil || id == uuid.Nil || id.String() != s.ID || (s.SchemaVersion != 1 && s.SchemaVersion != 2) ||
		s.Namespace == "" || s.Release == "" || s.APIServiceUID == "" ||
		s.CreatedAt.IsZero() || s.CreatedAt.After(now) ||
		!slices.Contains(rotationPhases, s.Phase) {
		return errors.New("invalid CA rotation journal identity or phase")
	}
	if s.SchemaVersion == 1 && s.Plan != nil && (s.Plan.Scope != "" || s.Plan.Aggregation != nil || s.Plan.ExternalIssuer != nil) ||
		s.SchemaVersion == 2 && (s.Plan == nil || s.Plan.Scope == "" || s.Plan.Aggregation == nil) {
		return errors.New("CA rotation scope does not match its journal version")
	}
	if len(s.PriorRevisions) > 8 || len(s.PriorRevisions) > 0 && (!s.Recovery || s.PreviousRevision == "") {
		return errors.New("invalid CA incident recovery predecessor history")
	}
	if s.PreviousRevision != "" {
		previousID, phase, found := strings.Cut(s.PreviousRevision, "/")
		previous, err := uuid.Parse(previousID)
		if !found || err != nil || previous == uuid.Nil || previous.String() != previousID ||
			previousID == s.ID || (!s.Recovery && phase != string(rotationComplete)) ||
			!slices.Contains(rotationPhases, rotationPhase(phase)) {
			return errors.New("invalid previous CA rotation revision")
		}
		for _, revision := range s.PriorRevisions {
			id, phase, ok := strings.Cut(revision, "/")
			parsed, err := uuid.Parse(id)
			if !ok || err != nil || parsed == uuid.Nil || parsed.String() != id || id == s.ID ||
				!slices.Contains(rotationPhases, rotationPhase(phase)) {
				return errors.New("invalid CA incident recovery predecessor revision")
			}
		}
	}
	// Historical trust can expire after retirement. Inspect its certificates
	// without permitting expired material in the current trust bundle.
	oldDigest, oldRoots, err := rotationHistoricalDigest(s.OldCA)
	if err != nil || (!s.Recovery && len(oldRoots) != 1) {
		return errors.New("rotation requires exactly one original dedicated or legacy aggregation CA")
	}
	newAt := now
	if s.Phase == rotationComplete {
		newAt = s.CreatedAt
	}
	newDigest, newRoots, err := mtls.TrustBundleDigest(s.NewCA, newAt)
	if err != nil || len(newRoots) != 1 || newDigest == oldDigest {
		return errors.New("rotation requires exactly one distinct, currently valid target CA")
	}
	if err := rotationCheckRootKeys(s.OldCA, s.NewCA); err != nil {
		return err
	}
	if s.Recovery && s.Phase != rotationRetire && s.Phase != rotationComplete {
		return errors.New("incident recovery must proceed directly toward verified new-only trust")
	}
	if !s.Recovery && s.Phase != rotationRetire && s.Phase != rotationComplete {
		if _, _, err := mtls.TrustBundleDigest(s.OldCA, now); err != nil {
			return errors.New("old CA expired before retirement; explicit incident recovery is required")
		}
	}
	if s.Plan != nil {
		if err := s.Plan.validate(); err != nil {
			return err
		}
		if s.Plan.effectiveScope() == rotationComponentsAndAggregation {
			if err := rotationCheckRootKeys(s.Plan.OldComponentCA, s.NewCA); err != nil {
				return err
			}
		}
		if s.Plan.TargetTrustSHA256 != newDigest || s.Plan.TargetTrustMap != "radius-mtls-ca-"+newDigest[:32] {
			return errors.New("rotation plan does not match the target public trust")
		}
		suffix := strings.ReplaceAll(s.ID, "-", "")[:16]
		servingName := s.Plan.ServingSecret == "radius-aggregation-"+suffix ||
			s.SchemaVersion == 2 && s.Plan.ServingSecret == "ucp-cert-aggregation-"+suffix
		if s.Plan.ExternalIssuer != nil {
			servingName = s.Plan.ServingSecret == "ucp-cert-aggregation-"+suffix
		}
		if (s.Plan.ExternalIssuer == nil && s.Plan.Issuer != "radius-ca-"+suffix) || !servingName {
			return errors.New("rotation issuer and serving Secret must be scoped to this transaction")
		}
	}
	return nil
}

func (p rotationPlan) validate() error {
	if !slices.Contains([]string{"selfSigned", "caSecret", "issuerRef"}, p.TargetMode) ||
		p.UCPMinReadySeconds < 0 || p.UCPProgressDeadlineSeconds <= p.UCPMinReadySeconds ||
		p.Issuer == "" || p.ServingSecret == "" || p.ServiceUID == "" || len(p.ServiceSelector) == 0 {
		return errors.New("invalid CA rotation resource plan")
	}
	if p.TargetMode == "issuerRef" {
		if p.ExternalIssuer == nil || p.SigningSecret != "" || p.SigningUID != "" || p.Issuer != p.ExternalIssuer.Name {
			return errors.New("issuerRef rotation must pin an existing public issuer, not a signing Secret")
		}
		if err := p.ExternalIssuer.validate(); err != nil {
			return err
		}
	} else if p.ExternalIssuer != nil || p.SigningUID == "" || p.SigningSecret == "" || len(validation.IsDNS1123Subdomain(p.SigningSecret)) != 0 {
		return errors.New("local-signer rotation requires the signing Secret identity")
	}
	if p.ServiceSelector["app.kubernetes.io/name"] != "ucp" || p.ServiceSelector["pod-template-hash"] != "" {
		return errors.New("invalid CA rotation signer name or aggregation selector")
	}
	if _, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: p.ServiceSelector}); err != nil {
		return errors.New("invalid CA rotation aggregation selector")
	}
	if err := p.validateScope(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, consumer := range p.Consumers {
		if !slices.Contains(rotationComponents, consumer.Name) || seen[consumer.Name] ||
			consumer.UID == "" || consumer.CertificateUID == "" || consumer.OldIssuer == "" || consumer.ProofPort == 0 {
			return errors.New("invalid or duplicate planned CA rotation workload")
		}
		if consumer.OldIssuerGroup != "" && consumer.OldIssuerGroup != "cert-manager.io" ||
			consumer.OldIssuerKind != "" && !slices.Contains([]string{"Issuer", "ClusterIssuer"}, consumer.OldIssuerKind) {
			return errors.New("invalid previous component issuer reference")
		}
		seen[consumer.Name] = true
	}
	return nil
}

func (s rotationState) bundle() []byte {
	if s.Phase == rotationRetire || s.Phase == rotationComplete {
		return bytes.Clone(s.NewCA)
	}
	bundle := append(bytes.Clone(bytes.TrimSpace(s.OldCA)), '\n')
	return append(bundle, s.NewCA...)
}

func (s rotationState) revision() string {
	return s.ID + "/" + string(s.Phase)
}

// rotationStore owns only the public journal and APIService trust. It never writes
// a signing Secret, installs admission policy, or takes control of an injector.
type rotationStore struct {
	kube               kubernetes.Interface
	dynamic            dynamic.Interface
	namespace          string
	release            string
	now                func() time.Time
	verifyIssuerPolicy func(context.Context, rotationState) error
}

func (s *rotationStore) read(ctx context.Context) (*corev1.ConfigMap, rotationState, error) {
	return s.readJournal(ctx, false)
}

func (s *rotationStore) readJournal(ctx context.Context, recovery bool) (*corev1.ConfigMap, rotationState, error) {
	journal, err := s.kube.CoreV1().ConfigMaps(s.namespace).Get(ctx, rotationJournalName, metav1.GetOptions{})
	if err != nil {
		return nil, rotationState{}, fmt.Errorf("cannot read CA rotation journal: %w", err)
	}
	if journal.Labels[rotationOwner] != s.release ||
		journal.DeletionTimestamp != nil || len(journal.OwnerReferences) != 0 ||
		journal.Annotations["meta.helm.sh/release-name"] != "" {
		return nil, rotationState{}, errors.New("CA rotation journal is not independently owned by this Radius release")
	}
	data := journal.Data[rotationJournalKey]
	if len(data) == 0 || len(data) > maxRotationBytes {
		return nil, rotationState{}, errors.New("CA rotation journal is missing or exceeds its size limit")
	}
	state, err := decodeRotationState(data)
	if err != nil {
		return nil, rotationState{}, err
	}
	if state.Namespace != s.namespace || state.Release != s.release {
		return nil, rotationState{}, errors.New("CA rotation journal belongs to a different Radius installation")
	}
	validation := state
	if recovery {
		if !slices.Contains(rotationPhases, state.Phase) || state.Recovery && state.Phase != rotationRetire && state.Phase != rotationComplete {
			return nil, rotationState{}, errors.New("invalid CA rotation journal phase")
		}
		// Inspection for explicit recovery validates historical material only.
		// No writer consumes this relaxed view as permission to publish trust.
		validation.Phase = rotationComplete
	}
	if err := validation.validate(s.now()); err != nil {
		return nil, rotationState{}, err
	}
	return journal, state, nil
}

func decodeRotationState(data string) (rotationState, error) {
	if len(data) > maxRotationBytes {
		return rotationState{}, errors.New("CA rotation journal exceeds its size limit")
	}
	decoder := json.NewDecoder(strings.NewReader(data))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return rotationState{}, errors.New("CA rotation journal must contain one object")
	}
	if err := checkRotationJSON(decoder, first, 0); err != nil {
		return rotationState{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return rotationState{}, errors.New("CA rotation journal contains trailing data")
	}
	var state rotationState
	decoder = json.NewDecoder(strings.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return rotationState{}, errors.New("invalid CA rotation journal schema")
	}
	return state, nil
}

func checkRotationJSON(decoder *json.Decoder, token json.Token, depth int) error {
	if depth > 16 || token == nil {
		return errors.New("invalid or null CA rotation journal field")
	}
	delim, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	fields := map[string]bool{}
	for decoder.More() {
		if delim == '{' {
			key, err := decoder.Token()
			if err != nil {
				return errors.New("invalid CA rotation journal")
			}
			field, ok := key.(string)
			if !ok || fields[field] {
				return errors.New("duplicate CA rotation journal field")
			}
			fields[field] = true
		}
		value, err := decoder.Token()
		if err != nil {
			return errors.New("invalid CA rotation journal")
		}
		if err := checkRotationJSON(decoder, value, depth+1); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return errors.New("invalid CA rotation journal")
	}
	return nil
}

func (s *rotationStore) begin(ctx context.Context, id string, target []byte) (rotationState, error) {
	return s.beginPlanned(ctx, id, target, nil)
}

func (s *rotationStore) beginPlanned(ctx context.Context, id string, target []byte, plan *rotationPlan) (rotationState, error) {
	api, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if err != nil {
		return rotationState{}, fmt.Errorf("cannot inspect Radius APIService: %w", err)
	}
	if err := s.checkAPIService(api); err != nil {
		return rotationState{}, err
	}
	current, err := apiServiceBundle(api)
	if err != nil {
		return rotationState{}, err
	}
	state := rotationState{
		SchemaVersion: 1, ID: id, Namespace: s.namespace, Release: s.release,
		APIServiceUID: api.GetUID(), Phase: rotationPublish, CreatedAt: s.now().UTC(),
		OldCA: current, NewCA: bytes.Clone(target), Plan: plan,
	}
	if plan != nil && plan.Scope != "" {
		state.SchemaVersion = 2
	}
	if err := state.validate(s.now()); err != nil {
		return rotationState{}, err
	}
	existing, previous, err := s.read(ctx)
	if err != nil && !apierrors.IsNotFound(err) {
		return rotationState{}, err
	}
	if err == nil {
		if previous.Phase != rotationComplete || previous.ID == id ||
			previous.APIServiceUID != api.GetUID() ||
			!bytes.Equal(current, previous.NewCA) ||
			api.GetAnnotations()[rotationRevision] != previous.revision() ||
			api.GetAnnotations()[rotationJournalUID] != string(existing.UID) {
			return rotationState{}, errors.New("an existing CA rotation must be completed and reconciled before starting another")
		}
		state.PreviousRevision = previous.revision()
	} else if api.GetAnnotations()[rotationJournalUID] != "" || api.GetAnnotations()[rotationRevision] != "" {
		return rotationState{}, errors.New("CA rotation journal is missing for previously managed trust; recover it instead of creating a replacement")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return rotationState{}, fmt.Errorf("cannot encode CA rotation journal: %w", err)
	}
	if len(data) > maxRotationBytes {
		return rotationState{}, errors.New("CA rotation journal exceeds its size limit")
	}
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: rotationJournalName, Namespace: s.namespace,
			Labels: map[string]string{rotationOwner: s.release},
		},
		Data: map[string]string{rotationJournalKey: string(data)},
	}
	if existing == nil {
		_, err = s.kube.CoreV1().ConfigMaps(s.namespace).Create(ctx, desired, metav1.CreateOptions{FieldManager: FieldManager})
	} else {
		// A new transaction can replace only an observed completed transaction.
		// There is no timeout-based takeover of an active writer.
		desired.UID, desired.ResourceVersion = existing.UID, existing.ResourceVersion
		_, err = s.kube.CoreV1().ConfigMaps(s.namespace).Update(ctx, desired, metav1.UpdateOptions{FieldManager: FieldManager})
	}
	if err != nil {
		return rotationState{}, fmt.Errorf("cannot acquire CA rotation journal; inspect and resume any existing transaction: %w", err)
	}
	return state, nil
}

func (s *rotationStore) checkAPIService(api *unstructured.Unstructured) error {
	if api.GetUID() == "" || api.GetResourceVersion() == "" || api.GetDeletionTimestamp() != nil {
		return errors.New("Radius APIService is missing its identity or is terminating")
	}
	if owner := api.GetAnnotations()[rotationOwner]; owner != "" && owner != s.release {
		return errors.New("Radius APIService trust belongs to another owner")
	}
	for _, annotation := range []string{
		"cert-manager.io/inject-ca-from", "cert-manager.io/inject-ca-from-secret", "cert-manager.io/inject-apiserver-ca",
	} {
		if _, exists := api.GetAnnotations()[annotation]; exists {
			return errors.New("Radius APIService has CA injection configured; a safe explicit handover is required, not annotation removal")
		}
	}
	for _, field := range api.GetManagedFields() {
		if field.Manager == "cert-manager-cainjector" {
			return errors.New("Radius APIService has injector write history; a safe explicit handover is required")
		}
	}
	if api.GetAnnotations()["meta.helm.sh/release-name"] != s.release ||
		api.GetAnnotations()["meta.helm.sh/release-namespace"] != s.namespace {
		return errors.New("Radius APIService is not owned by this Helm release")
	}
	namespace, _, err := unstructured.NestedString(api.Object, "spec", "service", "namespace")
	if err != nil {
		return errors.New("invalid Radius APIService service reference")
	}
	name, _, err := unstructured.NestedString(api.Object, "spec", "service", "name")
	if err != nil || namespace != s.namespace || name != "ucp" {
		return errors.New("Radius APIService points to an unexpected service")
	}
	insecure, _, err := unstructured.NestedBool(api.Object, "spec", "insecureSkipTLSVerify")
	if err != nil || insecure {
		return errors.New("Radius APIService must verify its serving certificate")
	}
	return nil
}

func apiServiceBundle(api *unstructured.Unstructured) ([]byte, error) {
	encoded, exists, err := unstructured.NestedString(api.Object, "spec", "caBundle")
	if err != nil || !exists {
		return nil, errors.New("Radius APIService has no public CA bundle")
	}
	bundle, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(bundle) == 0 || len(bundle) > maxRotationBytes {
		return nil, errors.New("Radius APIService has an invalid public CA bundle")
	}
	return bundle, nil
}

// reconcileTrust reads the target before the authoritative journal. A delayed
// write therefore carries an old target resourceVersion and cannot undo a later
// publication/retirement. On conflict the caller must restart this whole read
// sequence; it must never attach a fresh version to previously computed CA data.
func (s *rotationStore) reconcileTrust(ctx context.Context, transaction string) error {
	api, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("cannot inspect Radius APIService: %w", err)
	}
	if err := s.checkAPIService(api); err != nil {
		return err
	}
	journal, state, err := s.read(ctx)
	if err != nil {
		return err
	}
	if state.ID != transaction || api.GetUID() != state.APIServiceUID {
		return errors.New("CA rotation transaction or APIService identity changed")
	}
	if _, _, err := mtls.TrustBundleDigest(state.bundle(), s.now()); err != nil {
		return errors.New("the intended public trust is no longer valid; select a fresh CA with explicit incident recovery")
	}
	liveBundle, err := apiServiceBundle(api)
	if err != nil {
		return err
	}
	// Never silently repair unrelated edits. These are the only trust states
	// this transaction may have published, including an interrupted phase.
	overlap := state
	overlap.Phase = rotationPublish
	if !bytes.Equal(liveBundle, state.OldCA) && !bytes.Equal(liveBundle, overlap.bundle()) && !bytes.Equal(liveBundle, state.NewCA) &&
		!(state.Recovery && rotationHistoricalSubset(liveBundle, state.OldCA)) {
		return errors.New("Radius APIService trust changed outside the active rotation")
	}
	annotations := api.GetAnnotations()
	if owner := annotations[rotationOwner]; owner != "" && owner != s.release {
		return errors.New("Radius APIService trust belongs to another owner")
	}
	if uid := annotations[rotationJournalUID]; uid != "" && uid != string(journal.UID) {
		return errors.New("CA rotation journal was replaced; automatic trust adoption is forbidden")
	}
	if revision := annotations[rotationRevision]; revision != "" && !rotationPreviousRevision(revision, state) {
		phases := rotationPhases
		allowed := false
		for _, phase := range phases[:slices.Index(phases, state.Phase)+1] {
			if revision == state.ID+"/"+string(phase) {
				allowed = true
			}
		}
		if !allowed {
			return errors.New("CA rotation journal is older than or unrelated to published trust")
		}
	}
	annotations[rotationOwner] = s.release
	annotations[rotationRevision] = state.revision()
	annotations[rotationJournalUID] = string(journal.UID)
	api.SetAnnotations(annotations)
	if err := unstructured.SetNestedField(api.Object, base64.StdEncoding.EncodeToString(state.bundle()), "spec", "caBundle"); err != nil {
		return fmt.Errorf("cannot prepare Radius APIService trust: %w", err)
	}
	_, err = s.dynamic.Resource(apiServiceResource).Update(ctx, api, metav1.UpdateOptions{FieldManager: FieldManager})
	if err != nil {
		return fmt.Errorf("cannot publish Radius APIService trust; reread the transaction before retrying: %w", err)
	}
	return nil
}

// advance persists the next phase's intent using the observed journal identity
// and version. The caller establishes the current phase's external barriers.
func (s *rotationStore) advance(ctx context.Context, observed *corev1.ConfigMap, state rotationState, next rotationPhase) error {
	phases := rotationPhases
	index := slices.Index(phases, state.Phase)
	if index < 0 || index+1 == len(phases) || phases[index+1] != next {
		return errors.New("invalid CA rotation phase transition")
	}
	if observed == nil || observed.Name != rotationJournalName || observed.Namespace != s.namespace ||
		observed.UID == "" || observed.ResourceVersion == "" {
		return errors.New("CA rotation transition requires an observed journal identity and version")
	}
	stored, err := decodeRotationState(observed.Data[rotationJournalKey])
	if err != nil {
		return fmt.Errorf("invalid observed CA rotation journal: %w", err)
	}
	expected, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("cannot encode observed CA rotation state: %w", err)
	}
	actual, err := json.Marshal(stored)
	if err != nil || !bytes.Equal(actual, expected) {
		return errors.New("CA rotation state does not match the observed journal")
	}
	state.Phase = next
	if _, _, err := mtls.TrustBundleDigest(state.NewCA, s.now()); err != nil {
		return errors.New("target CA expired before the phase completed; select a fresh CA with explicit incident recovery")
	}
	if err := state.validate(s.now()); err != nil {
		return err
	}
	api, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("cannot verify published trust before advancing rotation: %w", err)
	}
	if err := s.checkAPIService(api); err != nil {
		return err
	}
	bundle, err := apiServiceBundle(api)
	if err != nil {
		return err
	}
	if api.GetUID() != stored.APIServiceUID || api.GetAnnotations()[rotationJournalUID] != string(observed.UID) ||
		api.GetAnnotations()[rotationRevision] != stored.revision() || !bytes.Equal(bundle, stored.bundle()) {
		return errors.New("current CA rotation phase has not published its expected APIService trust")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("cannot encode CA rotation phase: %w", err)
	}
	updated := observed.DeepCopy()
	updated.Data[rotationJournalKey] = string(data)
	_, err = s.kube.CoreV1().ConfigMaps(s.namespace).Update(ctx, updated, metav1.UpdateOptions{FieldManager: FieldManager})
	if err != nil {
		return fmt.Errorf("cannot advance CA rotation; inspect the journal before retrying: %w", err)
	}
	return nil
}
