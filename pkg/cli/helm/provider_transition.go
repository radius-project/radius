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
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
)

const (
	providerTransitionJournal = "radius-provider-transition"
	providerTransitionKey     = "state.json"
	providerTransitionMarker  = "radius.io/provider-transition"
	providerTransitionLock    = "radius-identity-operation"
)

type providerTransitionPhase string

const (
	providerStopping  providerTransitionPhase = "stopping"
	providerStaging   providerTransitionPhase = "staging"
	providerStarting  providerTransitionPhase = "starting"
	providerVerifying providerTransitionPhase = "verifying"
	providerComplete  providerTransitionPhase = "complete"
)

// These helpers are deliberately not connected to the CLI. Activation requires
// the CA driver to hold the same operation lock, plus real target-mode evidence.
type providerAggregationSnapshot struct {
	APIServiceUID types.UID `json:"apiServiceUID"`
	JournalUID    types.UID `json:"journalUID"`
	Revision      string    `json:"revision"`
	TrustSHA256   string    `json:"trustSHA256"`
	SignerSHA256  string    `json:"signerSHA256"`
	Scope         string    `json:"scope"`
	Complete      bool      `json:"complete"`
}

type providerTransitionWorkload struct {
	Name            string    `json:"name"`
	UID             types.UID `json:"uid"`
	ResourceVersion string    `json:"resourceVersion"`
	Replicas        int32     `json:"replicas"`
	SourceSHA256    string    `json:"sourceSHA256"`
	TargetSHA256    string    `json:"targetSHA256"`
	MetadataSHA256  string    `json:"metadataSHA256"`
	PreviousMarker  string    `json:"previousMarker,omitempty"`
}

type providerTransitionConfig struct {
	Name            string    `json:"name"`
	UID             types.UID `json:"uid"`
	ResourceVersion string    `json:"resourceVersion"`
	SHA256          string    `json:"sha256"`
}

// Persist hashes, never rendered pod specifications: inline environment values
// can contain credentials. A retry supplies the same independently validated specs.
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
}

type providerTransitionState struct {
	Version int                     `json:"version"`
	Plan    providerTransitionPlan  `json:"plan"`
	Phase   providerTransitionPhase `json:"phase"`
}

type providerTransitionChecks interface {
	aggregation(context.Context) (providerAggregationSnapshot, error)
	// preflight is read-only. It must validate installed and target provider/mode,
	// credentials, routing, image capability, issuer policy and unchanged serving
	// projection. Target ConfigMaps must already exist under separate names when
	// their contents differ: this helper never rewrites live shared configuration.
	preflight(context.Context, providerTransitionPlan, map[string]appsv1.DeploymentSpec) error
	// verify must obtain real target-provider and aggregate serving evidence.
	verify(context.Context, providerTransitionPlan) error
}

type providerTransitionStore struct {
	kube   kubernetes.Interface
	checks providerTransitionChecks
}

func providerHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func providerSpecHash(spec appsv1.DeploymentSpec) (string, error) {
	spec.Replicas = nil
	return providerHash(spec)
}

func providerMetadataHash(d *appsv1.Deployment) (string, error) {
	annotations := maps.Clone(d.Annotations)
	delete(annotations, providerTransitionMarker)
	// The Deployment controller updates this annotation during reconciliation.
	delete(annotations, "deployment.kubernetes.io/revision")
	return providerHash(struct {
		Labels      map[string]string
		Annotations map[string]string
		Owners      []metav1.OwnerReference
	}{d.Labels, annotations, d.OwnerReferences})
}

func providerDigestValid(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size && hex.EncodeToString(raw) == value
}

func (s providerTransitionState) validate() error {
	p := s.Plan
	id, err := uuid.Parse(p.ID)
	if err != nil || id == uuid.Nil || id.String() != p.ID || s.Version != 1 ||
		len(validation.IsDNS1123Label(p.Namespace)) != 0 || p.Release != radiusReleaseName ||
		!slices.Contains([]string{"radius", "external"}, p.Source) ||
		!slices.Contains([]string{"radius", "external"}, p.Target) || p.Source == p.Target ||
		!slices.Contains([]string{"dryRun", "enforce"}, p.Mode) ||
		!slices.Contains([]providerTransitionPhase{providerStopping, providerStaging, providerStarting, providerVerifying, providerComplete}, s.Phase) {
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
		!providerDigestValid(a.TrustSHA256) || !providerDigestValid(a.SignerSHA256) ||
		!slices.Contains([]string{"componentsAndAggregation", "aggregationOnly"}, a.Scope) {
		return errors.New("provider transition requires pinned managed aggregation identity")
	}
	if len(p.Workloads) != len(rotationComponents) {
		return errors.New("provider transition requires all five workloads")
	}
	seen := map[string]bool{}
	for _, w := range p.Workloads {
		if !slices.Contains(rotationComponents, w.Name) || seen[w.Name] || w.UID == "" ||
			w.ResourceVersion == "" || w.Replicas < 0 ||
			!providerDigestValid(w.SourceSHA256) || !providerDigestValid(w.TargetSHA256) ||
			!providerDigestValid(w.MetadataSHA256) || w.SourceSHA256 == w.TargetSHA256 {
			return errors.New("invalid provider transition workload")
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
			config.UID == "" || config.ResourceVersion == "" || !providerDigestValid(config.SHA256) {
			return errors.New("invalid or duplicate provider configuration pin")
		}
		seen[config.Name] = true
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

func providerConfigurationNames(spec corev1.PodSpec) []string {
	names := map[string]bool{}
	for _, volume := range spec.Volumes {
		if volume.ConfigMap != nil {
			names[volume.ConfigMap.Name] = true
		}
		if volume.Projected != nil {
			for _, source := range volume.Projected.Sources {
				if source.ConfigMap != nil {
					names[source.ConfigMap.Name] = true
				}
			}
		}
	}
	for _, containers := range [][]corev1.Container{spec.Containers, spec.InitContainers} {
		for _, container := range containers {
			for _, env := range container.Env {
				if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
					names[env.ValueFrom.ConfigMapKeyRef.Name] = true
				}
			}
			for _, env := range container.EnvFrom {
				if env.ConfigMapRef != nil {
					names[env.ConfigMapRef.Name] = true
				}
			}
		}
	}
	return slices.Sorted(maps.Keys(names))
}

func (s *providerTransitionStore) configuration(ctx context.Context, namespace, name string) (providerTransitionConfig, error) {
	cm, err := s.kube.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return providerTransitionConfig{}, err
	}
	if cm.DeletionTimestamp != nil {
		return providerTransitionConfig{}, errors.New("provider configuration is being deleted")
	}
	hash, err := providerHash(struct {
		Data       map[string]string
		BinaryData map[string][]byte
		Immutable  *bool
	}{cm.Data, cm.BinaryData, cm.Immutable})
	return providerTransitionConfig{Name: name, UID: cm.UID, ResourceVersion: cm.ResourceVersion, SHA256: hash}, err
}

func (s *providerTransitionStore) checkConfigurations(ctx context.Context, plan providerTransitionPlan) error {
	for _, pin := range plan.Configurations {
		current, err := s.configuration(ctx, plan.Namespace, pin.Name)
		if err != nil {
			return err
		}
		if current != pin {
			return fmt.Errorf("provider configuration %s changed during transition", pin.Name)
		}
	}
	return nil
}

func (s *providerTransitionStore) checkAggregation(ctx context.Context, plan providerTransitionPlan) error {
	if s.checks == nil {
		return errors.New("provider transition requires aggregation and target evidence checks")
	}
	got, err := s.checks.aggregation(ctx)
	if err != nil {
		return err
	}
	if got != plan.Aggregation {
		return errors.New("aggregation identity or revision changed during provider transition")
	}
	return nil
}

func (s *providerTransitionStore) read(ctx context.Context, namespace string) (*corev1.ConfigMap, providerTransitionState, error) {
	var state providerTransitionState
	cm, err := s.kube.CoreV1().ConfigMaps(namespace).Get(ctx, providerTransitionJournal, metav1.GetOptions{})
	if err != nil {
		return nil, state, err
	}
	raw := cm.Data[providerTransitionKey]
	if cm.UID == "" || cm.ResourceVersion == "" || cm.DeletionTimestamp != nil ||
		cm.Labels[providerTransitionMarker] != radiusReleaseName || len(cm.OwnerReferences) != 0 ||
		cm.Annotations["meta.helm.sh/release-name"] != "" || len(cm.Data) != 1 ||
		len(cm.BinaryData) != 0 || len(raw) > 64*1024 {
		return nil, state, errors.New("invalid provider transition journal ownership or content")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return nil, state, fmt.Errorf("invalid provider transition journal: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, state, errors.New("trailing provider transition journal content")
	}
	if err := state.validate(); err != nil {
		return nil, state, err
	}
	if state.Plan.Namespace != namespace {
		return nil, state, errors.New("provider transition namespace changed")
	}
	return cm, state, nil
}

func (s *providerTransitionStore) advance(ctx context.Context, lease *coordinationv1.Lease, cm *corev1.ConfigMap, state providerTransitionState, next providerTransitionPhase) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	phases := []providerTransitionPhase{providerStopping, providerStaging, providerStarting, providerVerifying, providerComplete}
	index := slices.Index(phases, state.Phase)
	if index < 0 || index+1 >= len(phases) || phases[index+1] != next {
		return errors.New("provider transition cannot skip or reverse a phase")
	}
	state.Phase = next
	if err := state.validate(); err != nil {
		return err
	}
	if err := s.checkAggregation(ctx, state.Plan); err != nil {
		return err
	}
	if err := s.checkConfigurations(ctx, state.Plan); err != nil {
		return err
	}
	if err := s.checkLock(ctx, lease); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	copy := cm.DeepCopy()
	copy.Data = map[string]string{providerTransitionKey: string(data)}
	_, err = s.kube.CoreV1().ConfigMaps(cm.Namespace).Update(ctx, copy, metav1.UpdateOptions{FieldManager: FieldManager})
	return err
}
