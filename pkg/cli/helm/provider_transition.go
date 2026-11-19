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
	"io"
	"slices"
	"strings"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	providerTransitionJournal = identityProviderJournal
	providerTransitionKey     = identityProviderStateKey
	providerTransitionMarker  = identityProviderMarker
)

type providerTransitionPhase string

const (
	providerStopping  providerTransitionPhase = "stopping"
	providerStaging   providerTransitionPhase = "staging"
	providerStarting  providerTransitionPhase = "starting"
	providerVerifying providerTransitionPhase = "verifying"
	providerRecording providerTransitionPhase = "recording"
	providerComplete  providerTransitionPhase = "complete"
)

type providerAggregationSnapshot = aggregationTransitionSnapshot

type providerTransitionWorkload struct {
	Name               string    `json:"name"`
	UID                types.UID `json:"uid"`
	ResourceVersion    string    `json:"resourceVersion"`
	Replicas           int32     `json:"replicas"`
	SourceSHA256       string    `json:"sourceSHA256"`
	TargetSHA256       string    `json:"targetSHA256"`
	MetadataSHA256     string    `json:"metadataSHA256"`
	HelmMetadataSHA256 string    `json:"helmMetadataSHA256,omitempty"`
	PreviousMarker     string    `json:"previousMarker,omitempty"`
}

type providerTransitionConfig struct {
	Name            string    `json:"name"`
	UID             types.UID `json:"uid"`
	ResourceVersion string    `json:"resourceVersion"`
	SHA256          string    `json:"sha256"`
}

type providerHelmIntent struct {
	SourceRevision       int      `json:"sourceRevision"`
	ChartSHA256          string   `json:"chartSHA256"`
	SourceValuesSHA256   string   `json:"sourceValuesSHA256"`
	SourceManifestSHA256 string   `json:"sourceManifestSHA256"`
	TargetValuesSHA256   string   `json:"targetValuesSHA256"`
	TargetConfigurations []string `json:"targetConfigurations"`
}

// Retain the saved v2 schema for read-only operation guards. Provider execution
// is deferred; accepting a complete document here does not authorize a transition.
type providerTransitionPlan struct {
	ID                     string                       `json:"id"`
	Namespace              string                       `json:"namespace"`
	Release                string                       `json:"release"`
	Source                 string                       `json:"source"`
	Target                 string                       `json:"target"`
	Mode                   string                       `json:"mode"`
	PreviousJournalUID     types.UID                    `json:"previousJournalUID,omitempty"`
	PreviousJournalVersion string                       `json:"previousJournalVersion,omitempty"`
	Aggregation            providerAggregationSnapshot  `json:"aggregation"`
	Workloads              []providerTransitionWorkload `json:"workloads"`
	Configurations         []providerTransitionConfig   `json:"configurations,omitempty"`
	Helm                   *providerHelmIntent          `json:"helm,omitempty"`
}

type providerTransitionState struct {
	Version int                     `json:"version"`
	Plan    providerTransitionPlan  `json:"plan"`
	Phase   providerTransitionPhase `json:"phase"`
}

func (s providerTransitionState) validate() error {
	p := s.Plan
	id, err := uuid.Parse(p.ID)
	if err != nil || id == uuid.Nil || id.String() != p.ID || s.Version != 2 ||
		len(validation.IsDNS1123Label(p.Namespace)) != 0 || p.Release != radiusReleaseName ||
		!slices.Contains([]string{"radius", "external"}, p.Source) ||
		!slices.Contains([]string{"radius", "external"}, p.Target) || p.Source == p.Target ||
		!slices.Contains([]string{"dryRun", "enforce"}, p.Mode) ||
		!slices.Contains([]providerTransitionPhase{providerStopping, providerStaging, providerStarting, providerVerifying, providerRecording, providerComplete}, s.Phase) {
		return errors.New("invalid provider transition identity, mode or phase")
	}
	if (p.PreviousJournalUID == "") != (p.PreviousJournalVersion == "") {
		return errors.New("provider transition predecessor requires both UID and resourceVersion")
	}
	a := p.Aggregation
	transaction, phase, found := strings.Cut(a.Revision, "/")
	aggregateID, aggregateErr := uuid.Parse(transaction)
	if !a.Complete || a.APIServiceUID == "" || a.JournalUID == "" || a.Revision == "" ||
		!found || phase != "complete" || aggregateErr != nil || aggregateID == uuid.Nil || aggregateID.String() != transaction ||
		!validCertificateHash(a.TrustSHA256) || !validCertificateHash(a.SignerSHA256) ||
		!slices.Contains([]rotationScope{rotationComponentsAndAggregation, rotationAggregationOnly}, a.Scope) {
		return errors.New("provider transition requires pinned managed aggregation identity")
	}
	workloads := []string{"ucp", "applications-rp", "dynamic-rp", "controller", "bicep-de"}
	if len(p.Workloads) != len(workloads) {
		return errors.New("provider transition requires all five workloads")
	}
	seen := map[string]bool{}
	for _, w := range p.Workloads {
		if !slices.Contains(workloads, w.Name) || seen[w.Name] || w.UID == "" ||
			w.ResourceVersion == "" || w.Replicas < 0 ||
			!validCertificateHash(w.SourceSHA256) || !validCertificateHash(w.TargetSHA256) ||
			!validCertificateHash(w.MetadataSHA256) || w.SourceSHA256 == w.TargetSHA256 {
			return errors.New("invalid provider transition workload")
		}
		if (p.Helm != nil || w.HelmMetadataSHA256 != "") && !validCertificateHash(w.HelmMetadataSHA256) {
			return errors.New("provider Helm recording requires exact workload metadata pins")
		}
		if w.PreviousMarker != "" {
			previous, phase, found := strings.Cut(w.PreviousMarker, "/")
			id, err := uuid.Parse(previous)
			if p.PreviousJournalUID == "" || !found || phase != string(providerStarting) ||
				err != nil || id == uuid.Nil || id.String() != previous || previous == p.ID {
				return errors.New("provider transition workload has no valid completed predecessor")
			}
		}
		seen[w.Name] = true
	}
	seen = map[string]bool{}
	for _, config := range p.Configurations {
		if len(validation.IsDNS1123Subdomain(config.Name)) != 0 || seen[config.Name] ||
			config.UID == "" || config.ResourceVersion == "" || !validCertificateHash(config.SHA256) {
			return errors.New("invalid or duplicate provider configuration pin")
		}
		seen[config.Name] = true
	}
	if s.Phase == providerRecording && p.Helm == nil {
		return errors.New("provider recording requires pinned Helm intent")
	}
	if h := p.Helm; h != nil {
		if h.SourceRevision < 1 || !validCertificateHash(h.ChartSHA256) ||
			!validCertificateHash(h.SourceValuesSHA256) || !validCertificateHash(h.SourceManifestSHA256) ||
			!validCertificateHash(h.TargetValuesSHA256) {
			return errors.New("invalid provider Helm intent")
		}
		targets := map[string]bool{}
		for _, name := range h.TargetConfigurations {
			if !seen[name] || targets[name] {
				return errors.New("provider Helm intent requires distinct pinned target configurations")
			}
			targets[name] = true
		}
		generation := strings.ReplaceAll(p.ID, "-", "")
		for _, base := range []string{"ucp-config", "applications-rp-config", "dynamic-rp-config", "controller-config", "bicep-de-config"} {
			if !targets[base+"-"+generation] {
				return errors.New("provider Helm intent is missing a generated target configuration")
			}
		}
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if len(data) > 64*1024 {
		return errors.New("provider transition journal exceeds the size limit")
	}
	return nil
}

// encoding/json's struct decoder accepts duplicate keys and null scalar fields.
// Reject both before decoding a journal that authorizes mutations.
func checkProviderTransitionJSON(raw string) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return errors.New("provider transition journal must contain one object")
	}
	if err := checkRotationJSON(decoder, first, 0); err != nil {
		return fmt.Errorf("invalid provider transition journal JSON: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing provider transition journal content")
	}
	return nil
}

func decodeProviderTransitionJournal(cm *corev1.ConfigMap, namespace, release string) (providerTransitionState, error) {
	var state providerTransitionState
	if cm == nil || cm.Name != providerTransitionJournal || cm.Namespace != namespace {
		return state, errors.New("provider transition journal identity changed")
	}
	raw := cm.Data[providerTransitionKey]
	if cm.UID == "" || cm.ResourceVersion == "" || cm.DeletionTimestamp != nil ||
		cm.Labels[providerTransitionMarker] != release || len(cm.OwnerReferences) != 0 ||
		cm.Annotations["meta.helm.sh/release-name"] != "" || len(cm.Data) != 1 ||
		len(cm.BinaryData) != 0 || len(raw) > 64*1024 {
		return state, fmt.Errorf("invalid provider transition journal %s ownership or content", providerTransitionJournal)
	}
	if err := checkProviderTransitionJSON(raw); err != nil {
		return state, err
	}
	var envelope struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || envelope.Version != 2 {
		return state, errors.New("legacy or unsupported provider journal: token-authenticated transitions require protocol v2; preserve historical v1 state and use the matching pre-token release for investigation; no automatic recovery or completed-state migration exists")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return state, fmt.Errorf("invalid provider transition journal: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return state, errors.New("trailing provider transition journal content")
	}
	if err := state.validate(); err != nil {
		return state, err
	}
	if state.Plan.Namespace != namespace || state.Plan.Release != release {
		return state, errors.New("provider transition namespace or release changed")
	}
	return state, nil
}

// validateCompletedProviderTransitionJournal validates the exact object observed
// by the shared operation guard, not a second GET that could race its snapshot.
// The guard must hold the operation lease and recheck this object's UID/version.
func validateCompletedProviderTransitionJournal(cm *corev1.ConfigMap, namespace, release string) error {
	state, err := decodeProviderTransitionJournal(cm, namespace, release)
	if err != nil {
		return err
	}
	if state.Phase != providerComplete {
		return errors.New("provider transition is unfinished; execution and recovery are unsupported in this release; preserve the journal for investigation before changing the CA")
	}
	return nil
}

func (i *Impl) completedProviderJournalValidator(namespace, release string) func(*corev1.ConfigMap) error {
	return func(cm *corev1.ConfigMap) error {
		if err := validateCompletedProviderTransitionJournal(cm, namespace, release); err != nil {
			return err
		}
		if i.validateCompletedProviderJournal != nil {
			return i.validateCompletedProviderJournal(cm)
		}
		return nil
	}
}

func (i *Impl) checkCompletedProviderJournal(ctx context.Context, store *rotationStore) error {
	journal, err := store.kube.CoreV1().ConfigMaps(store.namespace).Get(ctx, providerTransitionJournal, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot inspect provider transition journal: %w", err)
	}
	return i.completedProviderJournalValidator(store.namespace, store.release)(journal)
}
