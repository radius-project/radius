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
	"fmt"
	"maps"
	"reflect"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const certificateMutationOwner = "radius.io/certificate-lifecycle-mutation"

// Only hashes enter the public journal: a selected workload spec may contain
// credentials in environment variables. Desired specs must be rendered again.
type certificateMutationIntent struct {
	Phase                rotationPhase            `json:"phase"`
	Kind                 string                   `json:"kind"`
	Name                 string                   `json:"name"`
	Before               aggregationBootstrapPin  `json:"before"`
	BeforeMetadataSHA256 string                   `json:"beforeMetadataSHA256"`
	AfterSHA256          string                   `json:"afterSHA256"`
	Observed             *aggregationBootstrapPin `json:"observed,omitempty"`
}

func (intent certificateMutationIntent) resource() (schema.GroupVersionResource, error) {
	switch intent.Kind {
	case "Deployment":
		if !slices.Contains([]string{"ucp", "applications-rp", "dynamic-rp", "controller", "bicep-de"}, intent.Name) {
			return schema.GroupVersionResource{}, errors.New("certificate mutation names an unsupported workload")
		}
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, nil
	case "Certificate":
		if intent.Name == "ucp-cert" && intent.Phase == rotationIssue {
			return rotationCertificateResource, nil
		}
	}
	return schema.GroupVersionResource{}, errors.New("certificate mutations are restricted to Radius workloads and the aggregation leaf")
}

func (state certificateLifecycle) validateMutations() error {
	if len(state.Execution.Mutations) > 16 {
		return errors.New("too many certificate workload mutations")
	}
	seen := map[string]bool{}
	for _, intent := range state.Execution.Mutations {
		key := string(intent.Phase) + "/" + intent.Kind + "/" + intent.Name
		if seen[key] || !slices.Contains([]rotationPhase{rotationAdopt, rotationIssue}, intent.Phase) ||
			intent.Before.UID == "" || intent.Before.ResourceVersion == "" ||
			!validCertificateHash(intent.Before.SpecSHA256) || !validCertificateHash(intent.BeforeMetadataSHA256) ||
			!validCertificateHash(intent.AfterSHA256) {
			return errors.New("certificate mutation has invalid or duplicate phase/configuration pins")
		}
		if _, err := intent.resource(); err != nil {
			return err
		}
		if intent.Kind == "Certificate" && !state.Planes["aggregation"].Affected ||
			intent.Kind == "Deployment" && !state.Planes["internal"].Affected &&
				(intent.Name != "ucp" || !state.Planes["aggregation"].Affected) {
			return errors.New("certificate mutation would change an unaffected plane")
		}
		if intent.Observed != nil && (intent.Observed.UID != intent.Before.UID || intent.Observed.ResourceVersion == "" ||
			intent.Observed.SpecSHA256 != intent.AfterSHA256) {
			return errors.New("certificate mutation observation differs from its owned target")
		}
		if state.Phase == rotationComplete && intent.Observed == nil {
			return errors.New("completed certificate lifecycle has an unobserved workload mutation")
		}
		seen[key] = true
	}
	return nil
}

func checkNextCertificateMutations(before, next certificateLifecycle) error {
	previous, following := before.Execution.Mutations, next.Execution.Mutations
	if len(following) < len(previous) || len(following) > len(previous)+1 {
		return errors.New("certificate mutation intents are append-only, one at a time")
	}
	for index, intent := range previous {
		target := following[index]
		if intent.Observed != nil && !reflect.DeepEqual(intent.Observed, target.Observed) {
			return errors.New("observed certificate mutation cannot be rewritten")
		}
		intent.Observed, target.Observed = nil, nil
		if !reflect.DeepEqual(intent, target) {
			return errors.New("certificate mutation cannot change its durable before/after intent")
		}
	}
	if len(following) != len(previous) && (before.Phase != next.Phase || following[len(previous)].Phase != before.Phase) {
		return errors.New("certificate mutation must be recorded in its active phase")
	}
	return nil
}

func (w *certificateExecutionWriter) mutationMarker(intent certificateMutationIntent) string {
	return fmt.Sprintf("%s/%s/%s", w.journal.UID, w.state.ID, intent.Phase)
}

// Status and managedFields updates do not change the intended mutation. Pin
// generation and all other metadata so spec changes restored by another writer,
// ownership changes and metadata edits cannot be mistaken for status progress.
func certificateMutationMetadataDigest(object *unstructured.Unstructured) (string, error) {
	metadata, found, err := unstructured.NestedMap(object.Object, "metadata")
	if err != nil || !found {
		return "", errors.New("certificate workload has no valid metadata")
	}
	delete(metadata, "resourceVersion")
	delete(metadata, "managedFields")
	return bootstrapDigest(metadata)
}

func (w *certificateExecutionWriter) readMutation(ctx context.Context, intent certificateMutationIntent) (*unstructured.Unstructured, bool, error) {
	resource, err := intent.resource()
	if err != nil {
		return nil, false, err
	}
	object, err := w.store.dynamic.Resource(resource).Namespace(w.store.namespace).Get(ctx, intent.Name, metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}
	pin, err := bootstrapResourcePin(object)
	if err != nil {
		return nil, false, err
	}
	if pin.UID != intent.Before.UID || !rotationOwned(object, w.store.namespace, w.store.release) ||
		len(object.GetOwnerReferences()) != 0 || object.GetDeletionTimestamp() != nil {
		return nil, false, errors.New("certificate workload lost its pinned owned identity")
	}
	after := pin.SpecSHA256 == intent.AfterSHA256 && object.GetAnnotations()[certificateMutationOwner] == w.mutationMarker(intent)
	metadataHash, err := certificateMutationMetadataDigest(object)
	if err != nil {
		return nil, false, err
	}
	before := intent.Observed == nil && pin.SpecSHA256 == intent.Before.SpecSHA256 &&
		metadataHash == intent.BeforeMetadataSHA256
	if !before && !after {
		return nil, false, errors.New("certificate workload differs from its exact before or owned after configuration")
	}
	return object, after, nil
}

func (w *certificateExecutionWriter) checkMutations(ctx context.Context) error {
	seen := map[string]bool{}
	for index := len(w.state.Execution.Mutations) - 1; index >= 0; index-- {
		intent := w.state.Execution.Mutations[index]
		key := intent.Kind + "/" + intent.Name
		if seen[key] {
			continue
		}
		if _, _, err := w.readMutation(ctx, intent); err != nil {
			return err
		}
		seen[key] = true
	}
	return ctx.Err()
}

func (w *certificateExecutionWriter) applySelectedMutation(ctx context.Context, desired *unstructured.Unstructured) error {
	if w.observeLoaded == nil {
		return errCertificateObserverUnavailable
	}
	if desired == nil || desired.GetNamespace() != w.store.namespace || w.journal == nil ||
		!slices.Contains([]rotationPhase{rotationAdopt, rotationIssue}, w.state.Phase) {
		return errors.New("selected workload mutation requires an active adopt or issue phase")
	}
	if err := w.check(ctx); err != nil {
		return err
	}
	intent := certificateMutationIntent{Kind: desired.GetKind(), Name: desired.GetName(), Phase: w.state.Phase}
	resource, err := intent.resource()
	if err != nil {
		return err
	}
	intent.AfterSHA256, err = rotationIssuerSpecDigest(desired)
	if err != nil {
		return err
	}
	index := slices.IndexFunc(w.state.Execution.Mutations, func(record certificateMutationIntent) bool {
		return record.Kind == intent.Kind && record.Name == intent.Name && record.Phase == intent.Phase
	})
	if index < 0 {
		object, err := w.store.dynamic.Resource(resource).Namespace(w.store.namespace).Get(ctx, intent.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		intent.Before, err = bootstrapResourcePin(object)
		if err != nil {
			return err
		}
		intent.BeforeMetadataSHA256, err = certificateMutationMetadataDigest(object)
		if err != nil {
			return err
		}
		next, err := cloneCertificateLifecycle(w.state)
		if err != nil {
			return err
		}
		index = len(next.Execution.Mutations)
		next.Execution.Mutations = append(next.Execution.Mutations, intent)
		if err := w.save(ctx, next); err != nil {
			return err
		}
	} else {
		recorded := w.state.Execution.Mutations[index]
		if recorded.AfterSHA256 != intent.AfterSHA256 {
			return errors.New("re-rendered workload differs from its recorded target")
		}
		intent = recorded
	}
	object, applied, err := w.readMutation(ctx, intent)
	if err != nil {
		return err
	}
	if intent.Observed != nil {
		return nil
	}
	if !applied {
		if intent.Kind == "Certificate" {
			name, _, err := unstructured.NestedString(desired.Object, "spec", "secretName")
			if err != nil || name != w.state.Execution.ServingSecret {
				return errors.New("aggregation mutation differs from the planned serving Secret")
			}
			if _, err := w.store.kube.CoreV1().Secrets(w.store.namespace).Get(ctx, name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
				if err != nil {
					return err
				}
				return errors.New("planned aggregation serving Secret appeared before issuance; do not overwrite it")
			}
		}
		if w.state.Phase == rotationIssue {
			if err := w.observeLoaded(ctx, w.state); err != nil {
				return err
			}
		}
		if err := w.check(ctx); err != nil {
			return err
		}
		spec, found, err := unstructured.NestedMap(desired.Object, "spec")
		if err != nil || !found {
			return errors.New("selected workload has no valid desired specification")
		}
		object.Object["spec"] = spec
		annotations := maps.Clone(object.GetAnnotations())
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[certificateMutationOwner] = w.mutationMarker(intent)
		object.SetAnnotations(annotations)
		object, err = w.store.dynamic.Resource(resource).Namespace(w.store.namespace).
			Update(ctx, object, metav1.UpdateOptions{FieldManager: FieldManager})
		if err != nil {
			return err
		}
	}
	pin, err := bootstrapResourcePin(object)
	if err != nil {
		return err
	}
	if pin.UID != intent.Before.UID || pin.SpecSHA256 != intent.AfterSHA256 {
		return errors.New("workload admission changed the recorded target; preserve intent and investigate")
	}
	next, err := cloneCertificateLifecycle(w.state)
	if err != nil {
		return err
	}
	next.Execution.Mutations[index].Observed = &pin
	return w.save(ctx, next)
}
