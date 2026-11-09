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
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/authz/mtls"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const rotationRecover rotationPhase = "recover"

// Historical roots are evidence of prior intent, never a currently usable pool.
// Validate each at its own notBefore so expiry cannot make recovery unreadable.
func rotationHistoricalRoots(bundles ...[]byte) (map[string][]byte, error) {
	roots := map[string][]byte{}
	for _, bundle := range bundles {
		if len(bundle) == 0 || len(bundle) > maxRotationBytes {
			return nil, errors.New("invalid historical CA bundle size")
		}
		for len(bytes.TrimSpace(bundle)) != 0 {
			if !bytes.HasPrefix(bytes.TrimSpace(bundle), []byte("-----BEGIN CERTIFICATE-----")) {
				return nil, errors.New("historical CA bundle contains non-certificate data")
			}
			block, rest := pem.Decode(bundle)
			if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
				return nil, errors.New("invalid historical CA PEM")
			}
			certificate, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, errors.New("invalid historical CA certificate")
			}
			canonical := pem.EncodeToMemory(block)
			_, hashes, err := mtls.TrustBundleDigest(canonical, certificate.NotBefore)
			if err != nil || len(hashes) != 1 {
				return nil, errors.New("historical certificate is not an eligible dedicated CA")
			}
			roots[hashes[0]] = canonical
			if len(roots) > 8 {
				return nil, errors.New("incident recovery exceeds the bounded historical root set")
			}
			bundle = rest
		}
	}
	if len(roots) == 0 {
		return nil, errors.New("historical CA bundle is empty")
	}
	return roots, nil
}

func rotationHistoricalDigest(bundle []byte) (string, []string, error) {
	roots, err := rotationHistoricalRoots(bundle)
	if err != nil {
		return "", nil, err
	}
	hashes := slices.Sorted(maps.Keys(roots))
	digest := sha256.Sum256([]byte(strings.Join(hashes, "\n")))
	return hex.EncodeToString(digest[:]), hashes, nil
}

func rotationHistoricalSubset(bundle, allowed []byte) bool {
	actual, err := rotationHistoricalRoots(bundle)
	if err != nil {
		return false
	}
	expected, err := rotationHistoricalRoots(allowed)
	if err != nil {
		return false
	}
	for hash := range actual {
		if _, ok := expected[hash]; !ok {
			return false
		}
	}
	return true
}

func rotationCheckRootKeys(previous, next []byte) error {
	roots, err := rotationHistoricalRoots(previous)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(next)
	if block == nil {
		return errors.New("target CA is missing")
	}
	target, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return errors.New("invalid target CA")
	}
	for _, root := range roots {
		block, _ := pem.Decode(root)
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return err
		}
		if !bytes.Equal(certificate.Raw, target.Raw) && bytes.Equal(certificate.RawSubjectPublicKeyInfo, target.RawSubjectPublicKeyInfo) {
			return errors.New("a replacement CA must use a different signing key; a renewed certificate with the old key cannot retire that key")
		}
	}
	return nil
}

func rotationPreviousRevision(revision string, state rotationState) bool {
	if revision == state.PreviousRevision {
		return true
	}
	if !state.Recovery {
		return false
	}
	id, phase, ok := strings.Cut(revision, "/")
	index := slices.Index(rotationPhases, rotationPhase(phase))
	if !ok || index < 0 {
		return false
	}
	for _, previous := range append(slices.Clone(state.PriorRevisions), state.PreviousRevision) {
		priorID, priorPhase, _ := strings.Cut(previous, "/")
		if id == priorID && index <= slices.Index(rotationPhases, rotationPhase(priorPhase)) {
			return true
		}
	}
	return false
}

func (s *rotationStore) beginRecovery(ctx context.Context, id string, target []byte, plan *rotationPlan, observed *corev1.ConfigMap) (rotationState, error) {
	api, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if err != nil {
		return rotationState{}, err
	}
	if err := s.checkAPIService(api); err != nil {
		return rotationState{}, err
	}
	live, err := apiServiceBundle(api)
	if err != nil {
		return rotationState{}, err
	}
	journal, previous, err := s.readJournal(ctx, true)
	if err != nil && !apierrors.IsNotFound(err) {
		return rotationState{}, err
	}
	if (journal == nil) != (observed == nil) || journal != nil &&
		(journal.UID != observed.UID || journal.ResourceVersion != observed.ResourceVersion) {
		return rotationState{}, errors.New("CA transaction changed while planning incident recovery; repeat the entire read-only plan")
	}
	state := rotationState{SchemaVersion: 1, ID: id, Namespace: s.namespace, Release: s.release,
		APIServiceUID: api.GetUID(), Phase: rotationRetire, CreatedAt: s.now().UTC(), NewCA: bytes.Clone(target),
		Plan: plan, Recovery: true}
	if plan != nil && plan.Scope != "" {
		state.SchemaVersion = 2
	}
	history := [][]byte{live}
	if journal != nil {
		if err := checkRecoveryTarget(target, previous); err != nil {
			return rotationState{}, err
		}
		if api.GetUID() != previous.APIServiceUID ||
			api.GetAnnotations()[rotationJournalUID] != "" && api.GetAnnotations()[rotationJournalUID] != string(journal.UID) {
			return rotationState{}, errors.New("incident recovery cannot adopt a replaced APIService or public journal")
		}
		if previous.Phase == rotationComplete {
			if !bytes.Equal(live, previous.NewCA) {
				return rotationState{}, errors.New("completed public trust changed outside Radius")
			}
		} else {
			historical, err := rotationHistoricalRoots(previous.OldCA, previous.NewCA)
			if err != nil {
				return rotationState{}, err
			}
			for _, bundle := range historical {
				history = append(history, bundle)
			}
			allowed := bytes.Join(history[1:], nil)
			if !rotationHistoricalSubset(live, allowed) {
				return rotationState{}, errors.New("public trust changed outside the interrupted transaction")
			}
			if previous.Plan != nil && previous.Plan.effectiveScope() == rotationComponentsAndAggregation {
				history = append(history, previous.Plan.OldComponentCA)
			}
			state.PriorRevisions = slices.Clone(previous.PriorRevisions)
			if previous.PreviousRevision != "" {
				state.PriorRevisions = append(state.PriorRevisions, previous.PreviousRevision)
			}
		}
		state.PreviousRevision = previous.revision()
	} else if api.GetAnnotations()[rotationJournalUID] != "" || api.GetAnnotations()[rotationRevision] != "" {
		return rotationState{}, errors.New("restore the missing public journal before incident recovery")
	}
	roots, err := rotationHistoricalRoots(history...)
	if err != nil {
		return rotationState{}, err
	}
	for _, hash := range slices.Sorted(maps.Keys(roots)) {
		state.OldCA = append(state.OldCA, roots[hash]...)
	}
	if err := state.validate(s.now()); err != nil {
		return rotationState{}, err
	}
	encoded, err := json.Marshal(state)
	if err != nil || len(encoded) > maxRotationBytes {
		return rotationState{}, errors.New("incident recovery journal exceeds its supported schema or size")
	}
	desired := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: rotationJournalName, Namespace: s.namespace,
		Labels: map[string]string{rotationOwner: s.release}}, Data: map[string]string{rotationJournalKey: string(encoded)}}
	if err := s.checkWrite(ctx); err != nil {
		return rotationState{}, err
	}
	if journal == nil {
		_, err = s.kube.CoreV1().ConfigMaps(s.namespace).Create(ctx, desired, metav1.CreateOptions{FieldManager: FieldManager})
	} else {
		desired.UID, desired.ResourceVersion = journal.UID, journal.ResourceVersion
		_, err = s.kube.CoreV1().ConfigMaps(s.namespace).Update(ctx, desired, metav1.UpdateOptions{FieldManager: FieldManager})
	}
	if err != nil {
		return rotationState{}, fmt.Errorf("cannot record explicit incident recovery; inspect the current journal before retrying: %w", err)
	}
	return state, nil
}

func checkRecoveryTarget(target []byte, previous rotationState) error {
	if previous.ID != "" && !rotationHistoricalSubset(target, previous.NewCA) {
		if rotationHistoricalSubset(target, previous.OldCA) ||
			previous.Plan != nil && previous.Plan.effectiveScope() == rotationComponentsAndAggregation && rotationHistoricalSubset(target, previous.Plan.OldComponentCA) {
			return errors.New("incident recovery cannot restore a previous CA; select a fresh signing CA or resume the interrupted target")
		}
		if previous.Plan != nil && previous.Plan.effectiveScope() == rotationComponentsAndAggregation {
			return rotationCheckRootKeys(previous.Plan.OldComponentCA, target)
		}
	}
	return nil
}

func (s *rotationStore) reconcileRecoveryRoute(ctx context.Context, transaction string) error {
	service, err := s.kube.CoreV1().Services(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return err
	}
	state, err := s.plannedPhase(ctx, transaction, rotationRetire)
	if err != nil {
		return err
	}
	if !state.Recovery || service.UID != state.Plan.ServiceUID || !rotationOwned(service, s.namespace, s.release) {
		return errors.New("incident recovery requires its owned aggregation Service")
	}
	if err := checkRotationObjectRevision(service, state); err != nil {
		return err
	}
	if err := checkRecoveryServiceSelector(service, state); err != nil {
		return err
	}
	service.Spec.Selector = maps.Clone(state.Plan.ServiceSelector)
	service.Annotations[rotationTransaction] = state.ID
	service.Annotations[rotationRevision] = state.revision()
	service.Annotations[rotationRoute] = "complete"
	delete(service.Annotations, rotationOldHash)
	delete(service.Annotations, rotationNewHash)
	if err := s.checkWrite(ctx); err != nil {
		return err
	}
	_, err = s.kube.CoreV1().Services(s.namespace).Update(ctx, service, metav1.UpdateOptions{FieldManager: FieldManager})
	return err
}

func checkRecoveryServiceSelector(service *corev1.Service, state rotationState) error {
	if err := checkRotationObjectRevision(service, state); err != nil {
		return err
	}
	if maps.Equal(service.Spec.Selector, state.Plan.ServiceSelector) {
		return nil
	}
	id, _, ok := strings.Cut(service.Annotations[rotationRevision], "/")
	if ok && id == service.Annotations[rotationTransaction] {
		key := ""
		switch service.Annotations[rotationRoute] {
		case "old":
			key = rotationOldHash
		case "new":
			key = rotationNewHash
		}
		if hash := service.Annotations[key]; key != "" && hash != "" {
			expected := maps.Clone(state.Plan.ServiceSelector)
			expected[appsv1.DefaultDeploymentUniqueLabelKey] = hash
			if maps.Equal(service.Spec.Selector, expected) {
				return nil
			}
		}
	}
	return errors.New("aggregation Service selector changed outside the rotation; incident recovery will not overwrite it")
}
