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
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	aggregationBootstrapName  = "radius-aggregation-bootstrap"
	bootstrapNamespaceIntent  = "radius.io/aggregation-bootstrap-intent"
	aggregationBootstrapOwner = "radius.io/aggregation-bootstrap"
	bootstrapPreparing        = "preparing"
	bootstrapInstalling       = "installing"
	bootstrapVerifying        = "verifying"
	bootstrapComplete         = "complete"
)

// Bootstrap records creation intent before an APIService or workload UID exists.
// It is distinct from rotation: there is no predecessor CA or serving fleet.
type aggregationBootstrapState struct {
	Version             int                                `json:"version"`
	ID                  string                             `json:"id"`
	Namespace           string                             `json:"namespace"`
	Release             string                             `json:"release"`
	Phase               string                             `json:"phase"`
	CreatedAt           time.Time                          `json:"createdAt"`
	Scope               rotationScope                      `json:"scope"`
	Mode                string                             `json:"mode"`
	IntentSHA256        string                             `json:"intentSHA256"`
	ChartSHA256         string                             `json:"chartSHA256"`
	InstallValuesSHA256 string                             `json:"installValuesSHA256,omitempty"`
	TrustSHA256         string                             `json:"trustSHA256,omitempty"`
	ProofPort           uint16                             `json:"proofPort,omitempty"`
	CA                  []byte                             `json:"ca,omitempty"`
	CAPinnedAt          time.Time                          `json:"caPinnedAt,omitzero"`
	Signer              string                             `json:"signer,omitempty"`
	SignerUID           types.UID                          `json:"signerUID,omitempty"`
	RootSecretUID       types.UID                          `json:"rootSecretUID,omitempty"`
	External            *rotationExternalIssuer            `json:"external,omitempty"`
	Resources           map[string]aggregationBootstrapPin `json:"resources"`
}

type aggregationBootstrapPin struct {
	UID             types.UID `json:"uid"`
	ResourceVersion string    `json:"resourceVersion"`
	SpecSHA256      string    `json:"specSHA256"`
}

func bootstrapDigest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func validBootstrapDigest(value string) bool {
	data, err := hex.DecodeString(value)
	return err == nil && len(data) == sha256.Size && strings.ToLower(value) == value
}

func (s aggregationBootstrapState) validate(now time.Time) error {
	id, err := uuid.Parse(s.ID)
	if err != nil || id == uuid.Nil || id.String() != s.ID || s.Version != 1 ||
		s.Namespace == "" || s.Release != radiusReleaseName || !validBootstrapDigest(s.IntentSHA256) || !validBootstrapDigest(s.ChartSHA256) ||
		s.CreatedAt.IsZero() || s.CreatedAt.After(now) ||
		s.Scope != rotationComponentsAndAggregation && s.Scope != rotationAggregationOnly ||
		s.Resources == nil || len(s.Resources) > 24 {
		return errors.New("invalid aggregation bootstrap identity or intent")
	}
	switch s.Phase {
	case bootstrapPreparing, bootstrapInstalling, bootstrapVerifying, bootstrapComplete:
	default:
		return errors.New("invalid aggregation bootstrap phase")
	}
	switch s.Mode {
	case "selfSigned", "caSecret":
		if s.External != nil || s.Signer == "" && s.SignerUID != "" || s.Mode == "caSecret" && s.Signer == "" {
			return errors.New("local bootstrap has inconsistent signing identity")
		}
	case "issuerRef":
		if s.External == nil || s.Signer != "" || s.SignerUID != "" {
			return errors.New("external bootstrap must pin a public issuer, not its private key")
		}
		if err := s.External.validate(); err != nil {
			return err
		}
	default:
		return errors.New("invalid aggregation bootstrap certificate mode")
	}
	if s.Mode != "selfSigned" && s.RootSecretUID != "" ||
		s.Mode == "selfSigned" && (len(s.CA) > 0) != (s.RootSecretUID != "") {
		return errors.New("selfSigned bootstrap must pin its observed root Secret identity with its CA")
	}
	if len(s.CA) == 0 {
		if s.Mode != "selfSigned" || s.Phase != bootstrapPreparing || s.Signer != "" || !s.CAPinnedAt.IsZero() {
			return errors.New("bootstrap requires its selected public CA")
		}
	} else {
		if s.CAPinnedAt.IsZero() || s.CAPinnedAt.Before(s.CreatedAt) || s.CAPinnedAt.After(now) {
			return errors.New("bootstrap requires the observed public CA pin time")
		}
		at := now
		if s.Phase == bootstrapComplete {
			at = s.CAPinnedAt
		}
		if _, roots, err := mtls.TrustBundleDigest(s.CA, at); err != nil || len(roots) != 1 {
			return errors.New("bootstrap requires one valid dedicated public CA")
		}
	}
	if s.Phase != bootstrapPreparing && s.External == nil && s.SignerUID == "" {
		return errors.New("bootstrap has no pinned signing identity")
	}
	if s.Phase != bootstrapPreparing {
		digest, _, err := mtls.TrustBundleDigest(s.CA, s.CAPinnedAt)
		if err != nil || digest != s.TrustSHA256 || s.ProofPort == 0 || !validBootstrapDigest(s.InstallValuesSHA256) {
			return errors.New("bootstrap has no pinned installation settings or serving trust")
		}
	}
	for name, resource := range s.Resources {
		if name == "" || resource.UID == "" || resource.ResourceVersion == "" || !validBootstrapDigest(resource.SpecSHA256) {
			return errors.New("bootstrap contains an incomplete observed resource identity")
		}
	}
	return nil
}

func decodeAggregationBootstrap(raw string) (aggregationBootstrapState, error) {
	var state aggregationBootstrapState
	if len(raw) == 0 || len(raw) > maxRotationBytes {
		return state, errors.New("aggregation bootstrap exceeds its schema size limit")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return state, errors.New("aggregation bootstrap must contain an object")
	}
	if err := checkRotationJSON(decoder, first, 0); err != nil {
		return state, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return state, errors.New("trailing aggregation bootstrap content")
	}
	decoder = json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return state, fmt.Errorf("invalid aggregation bootstrap: %w", err)
	}
	return state, nil
}

func (s *rotationStore) readBootstrap(ctx context.Context) (*corev1.ConfigMap, aggregationBootstrapState, error) {
	journal, err := s.kube.CoreV1().ConfigMaps(s.namespace).Get(ctx, aggregationBootstrapName, metav1.GetOptions{})
	if err != nil {
		return nil, aggregationBootstrapState{}, err
	}
	if journal.UID == "" || journal.ResourceVersion == "" || journal.DeletionTimestamp != nil ||
		journal.Labels[aggregationBootstrapOwner] != s.release || len(journal.OwnerReferences) != 0 ||
		journal.Annotations["meta.helm.sh/release-name"] != "" || len(journal.Data) != 1 || len(journal.BinaryData) != 0 {
		return nil, aggregationBootstrapState{}, errors.New("aggregation bootstrap journal changed ownership or content")
	}
	state, err := decodeAggregationBootstrap(journal.Data[rotationJournalKey])
	if err != nil {
		return nil, state, err
	}
	if state.Namespace != s.namespace || state.Release != s.release {
		return nil, state, errors.New("aggregation bootstrap belongs to another installation")
	}
	if err := state.validate(s.now()); err != nil {
		return nil, state, err
	}
	return journal, state, nil
}

func (s *rotationStore) bootstrapIntent(ctx context.Context) (aggregationBootstrapState, bool, error) {
	_, state, err := s.readBootstrap(ctx)
	if !apierrors.IsNotFound(err) {
		return state, err == nil, err
	}
	namespace, err := s.kube.CoreV1().Namespaces().Get(ctx, s.namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return state, false, nil
	}
	if err != nil {
		return state, false, err
	}
	raw := namespace.Annotations[bootstrapNamespaceIntent]
	if raw == "" {
		return state, false, nil
	}
	state, err = decodeAggregationBootstrap(raw)
	if err != nil {
		return state, false, err
	}
	if state.Namespace != s.namespace || state.Release != s.release || state.Phase != bootstrapPreparing ||
		len(state.Resources) != 0 || namespace.DeletionTimestamp != nil {
		return state, false, errors.New("namespace retains an invalid bootstrap creation intent")
	}
	return state, true, state.validate(s.now())
}

func (s *rotationStore) beginBootstrap(ctx context.Context, requested aggregationBootstrapState) (*corev1.ConfigMap, aggregationBootstrapState, error) {
	if requested.Phase != bootstrapPreparing || len(requested.Resources) != 0 {
		return nil, aggregationBootstrapState{}, errors.New("bootstrap must start with creation intent and no invented resource identities")
	}
	if err := requested.validate(s.now()); err != nil {
		return nil, aggregationBootstrapState{}, err
	}
	current, state, err := s.readBootstrap(ctx)
	if err == nil {
		if state.IntentSHA256 != requested.IntentSHA256 || state.ChartSHA256 != requested.ChartSHA256 || state.Mode != requested.Mode || state.Scope != requested.Scope {
			return nil, state, errors.New("bootstrap retry differs from the recorded chart or certificate intent")
		}
		if len(requested.CA) > 0 && !reflect.DeepEqual(requested.CA, state.CA) ||
			requested.Signer != "" && requested.Signer != state.Signer ||
			requested.SignerUID != "" && state.SignerUID != "" && requested.SignerUID != state.SignerUID ||
			!reflect.DeepEqual(requested.External, state.External) {
			return nil, state, errors.New("bootstrap retry changed its selected CA or issuer")
		}
		return current, state, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, state, err
	}
	if _, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		if err != nil {
			return nil, state, err
		}
		return nil, state, errors.New("fresh bootstrap cannot adopt an existing aggregation APIService")
	}
	if _, err := s.kube.CoreV1().Services(s.namespace).Get(ctx, "ucp", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		if err != nil {
			return nil, state, err
		}
		return nil, state, errors.New("fresh bootstrap cannot adopt an existing UCP Service")
	}
	for _, name := range rotationComponents {
		if _, err := s.kube.AppsV1().Deployments(s.namespace).Get(ctx, name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			if err != nil {
				return nil, state, err
			}
			return nil, state, errors.New("fresh bootstrap cannot adopt an existing Radius deployment")
		}
	}
	if _, _, err := s.read(ctx); !apierrors.IsNotFound(err) {
		if err != nil {
			return nil, state, err
		}
		return nil, state, errors.New("fresh bootstrap cannot replace an existing CA lifecycle journal")
	}
	data, err := json.Marshal(requested)
	if err != nil || len(data) > maxRotationBytes {
		return nil, state, errors.New("bootstrap intent exceeds its schema size limit")
	}
	if err := s.checkWrite(ctx); err != nil {
		return nil, state, err
	}
	current, err = s.kube.CoreV1().ConfigMaps(s.namespace).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: aggregationBootstrapName, Namespace: s.namespace,
			Labels: map[string]string{aggregationBootstrapOwner: s.release}},
		Data: map[string]string{rotationJournalKey: string(data)},
	}, metav1.CreateOptions{FieldManager: FieldManager})
	if err != nil {
		return nil, state, fmt.Errorf("cannot record bootstrap intent; reread before retrying: %w", err)
	}
	return current, requested, nil
}

func (s *rotationStore) updateBootstrap(ctx context.Context, observed *corev1.ConfigMap, previous, next aggregationBootstrapState) (*corev1.ConfigMap, error) {
	if observed == nil || observed.UID == "" || observed.ResourceVersion == "" ||
		previous.ID != next.ID || previous.IntentSHA256 != next.IntentSHA256 || previous.ChartSHA256 != next.ChartSHA256 ||
		previous.Mode != next.Mode || previous.Scope != next.Scope ||
		previous.Namespace != next.Namespace || previous.Release != next.Release ||
		previous.CreatedAt != next.CreatedAt {
		return nil, errors.New("bootstrap update changed its original intent")
	}
	phases := map[string]int{bootstrapPreparing: 0, bootstrapInstalling: 1, bootstrapVerifying: 2, bootstrapComplete: 3}
	from, exists := phases[previous.Phase]
	to, valid := phases[next.Phase]
	if !exists || !valid || to < from || to > from+1 || previous.Phase == bootstrapComplete {
		return nil, errors.New("invalid bootstrap phase transition")
	}
	if len(previous.CA) != 0 && (!reflect.DeepEqual(previous.CA, next.CA) || previous.CAPinnedAt != next.CAPinnedAt) ||
		previous.Signer != "" && previous.Signer != next.Signer ||
		previous.RootSecretUID != "" && previous.RootSecretUID != next.RootSecretUID ||
		previous.SignerUID != "" && (previous.SignerUID != next.SignerUID || previous.Signer != next.Signer) ||
		previous.InstallValuesSHA256 != "" && previous.InstallValuesSHA256 != next.InstallValuesSHA256 ||
		previous.TrustSHA256 != "" && previous.TrustSHA256 != next.TrustSHA256 ||
		previous.ProofPort != 0 && previous.ProofPort != next.ProofPort ||
		previous.External != nil && !reflect.DeepEqual(previous.External, next.External) {
		return nil, errors.New("bootstrap cannot replace a pinned CA or issuer")
	}
	for name, pin := range previous.Resources {
		if next.Resources[name] != pin {
			return nil, errors.New("bootstrap cannot replace a previously observed resource identity")
		}
	}
	if err := next.validate(s.now()); err != nil {
		return nil, err
	}
	live, state, err := s.readBootstrap(ctx)
	if err != nil {
		return nil, err
	}
	if live.UID != observed.UID || live.ResourceVersion != observed.ResourceVersion || !reflect.DeepEqual(state, previous) {
		return nil, errors.New("bootstrap intent changed; reread before retrying")
	}
	data, err := json.Marshal(next)
	if err != nil || len(data) > maxRotationBytes {
		return nil, errors.New("bootstrap update exceeds its schema size limit")
	}
	desired := observed.DeepCopy()
	desired.Data[rotationJournalKey] = string(data)
	if err := s.checkWrite(ctx); err != nil {
		return nil, err
	}
	return s.kube.CoreV1().ConfigMaps(s.namespace).Update(ctx, desired, metav1.UpdateOptions{FieldManager: FieldManager})
}
