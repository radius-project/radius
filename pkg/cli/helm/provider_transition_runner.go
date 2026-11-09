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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

var errProviderTransitionWaiting = errors.New("provider transition is waiting for workload convergence; retry the same transaction")

// withLock never steals an expired lease. A crashed holder requires explicit
// operator investigation; an elapsed wall clock is not evidence that it stopped.
func (s *providerTransitionStore) withLock(ctx context.Context, namespace string, run func(*coordinationv1.Lease) error) (err error) {
	return withIdentityOperation(ctx, s.kube, namespace, func(operation *identityOperation) error {
		return run(operation.lease)
	})
}

func (s *providerTransitionStore) checkLock(ctx context.Context, lease *coordinationv1.Lease) error {
	return (&identityOperation{kube: s.kube, lease: lease}).check(ctx)
}

func (s *providerTransitionStore) checkAutoscalers(ctx context.Context, plan providerTransitionPlan) error {
	list, err := s.kube.AutoscalingV2().HorizontalPodAutoscalers(plan.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, hpa := range list.Items {
		for _, w := range plan.Workloads {
			if hpa.Spec.ScaleTargetRef.Kind == "Deployment" && hpa.Spec.ScaleTargetRef.Name == w.Name {
				return fmt.Errorf("provider transition cannot quiesce autoscaled workload %s", w.Name)
			}
		}
	}
	return nil
}

func (s *providerTransitionStore) deployment(ctx context.Context, plan providerTransitionPlan, w providerTransitionWorkload) (*appsv1.Deployment, string, error) {
	d, err := s.kube.AppsV1().Deployments(plan.Namespace).Get(ctx, w.Name, metav1.GetOptions{})
	if err != nil {
		return nil, "", err
	}
	metadata, err := providerMetadataHash(d)
	if err != nil {
		return nil, "", err
	}
	digest, err := providerSpecHash(d.Spec)
	if err != nil {
		return nil, "", err
	}
	recorded := plan.Helm != nil && metadata == w.HelmMetadataSHA256 &&
		digest == w.TargetSHA256 && d.Annotations[providerTransitionMarker] == providerMarker(plan, providerStarting)
	if d.UID != w.UID || d.ResourceVersion == "" || d.DeletionTimestamp != nil || d.Spec.Paused ||
		!rotationOwned(d, plan.Namespace, plan.Release) || metadata != w.MetadataSHA256 && !recorded ||
		d.Spec.Replicas == nil || *d.Spec.Replicas < 0 {
		return nil, "", fmt.Errorf("provider transition workload %s identity or metadata drifted", w.Name)
	}
	return d, digest, nil
}

func (s *providerTransitionStore) validateTargets(ctx context.Context, state providerTransitionState, targets map[string]appsv1.DeploymentSpec) error {
	if err := state.validate(); err != nil {
		return err
	}
	if len(targets) != len(state.Plan.Workloads) {
		return errors.New("provider transition target inventory changed")
	}
	pinned := map[string]bool{}
	for _, config := range state.Plan.Configurations {
		pinned[config.Name] = true
	}
	for _, w := range state.Plan.Workloads {
		spec, ok := targets[w.Name]
		if !ok || spec.Paused || spec.Replicas == nil || *spec.Replicas != w.Replicas {
			return errors.New("provider transition target replicas or pause state changed")
		}
		digest, err := providerSpecHash(spec)
		if err != nil {
			return err
		}
		if digest != w.TargetSHA256 {
			return errors.New("provider transition rendered target changed")
		}
		d, _, err := s.deployment(ctx, state.Plan, w)
		if err != nil {
			return err
		}
		if !apiequality.Semantic.DeepEqual(spec.Selector, d.Spec.Selector) {
			return errors.New("provider transition cannot change workload selectors")
		}
		for _, pod := range []corev1.PodSpec{spec.Template.Spec, d.Spec.Template.Spec} {
			for _, name := range providerConfigurationNames(pod) {
				if !pinned[name] {
					return fmt.Errorf("provider configuration %s has not been pinned", name)
				}
			}
		}
	}
	if err := s.checkAggregation(ctx, state.Plan); err != nil {
		return err
	}
	if err := s.checkAutoscalers(ctx, state.Plan); err != nil {
		return err
	}
	if err := s.checkStateConfigurations(ctx, state); err != nil {
		return err
	}
	return s.checks.preflight(ctx, state.Plan, targets)
}

func (s *providerTransitionStore) checkInitial(ctx context.Context, plan providerTransitionPlan) error {
	for _, w := range plan.Workloads {
		d, hash, err := s.deployment(ctx, plan, w)
		if err != nil {
			return err
		}
		if d.ResourceVersion != w.ResourceVersion || hash != w.SourceSHA256 ||
			*d.Spec.Replicas != w.Replicas || d.Annotations[providerTransitionMarker] != w.PreviousMarker {
			return fmt.Errorf("provider transition source %s changed since planning", w.Name)
		}
	}
	return nil
}

func (s *providerTransitionStore) predecessor(ctx context.Context, plan providerTransitionPlan) (*corev1.ConfigMap, error) {
	cm, state, err := s.read(ctx, plan.Namespace)
	if apierrors.IsNotFound(err) && plan.PreviousJournalUID == "" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if state.Phase != providerComplete || state.Plan.Target != plan.Source ||
		state.Plan.ID == plan.ID || cm.UID != plan.PreviousJournalUID || cm.ResourceVersion != plan.PreviousJournalVersion {
		return nil, errors.New("provider transition requires an explicit unchanged completed predecessor")
	}
	for _, w := range plan.Workloads {
		if w.PreviousMarker != providerMarker(state.Plan, providerStarting) {
			return nil, errors.New("provider transition workload predecessor marker changed")
		}
	}
	return cm, nil
}

func (s *providerTransitionStore) plan(ctx context.Context, namespace, source, target, mode string, targets map[string]appsv1.DeploymentSpec) (providerTransitionPlan, error) {
	return s.planWithID(ctx, namespace, source, target, mode, uuid.NewString(), targets)
}

func (s *providerTransitionStore) planWithID(ctx context.Context, namespace, source, target, mode, id string, targets map[string]appsv1.DeploymentSpec) (providerTransitionPlan, error) {
	p := providerTransitionPlan{ID: id, Namespace: namespace, Release: radiusReleaseName, Source: source, Target: target, Mode: mode}
	if s.checks == nil {
		return p, errors.New("provider transition requires aggregation and target evidence checks")
	}
	snapshot, err := s.checks.aggregation(ctx)
	if err != nil {
		return p, err
	}
	p.Aggregation = snapshot
	configurations := map[string]bool{}
	for _, name := range rotationComponents {
		d, err := s.kube.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return p, err
		}
		spec, found := targets[name]
		if !found || d.Spec.Replicas == nil {
			return p, errors.New("provider transition requires an explicit source and target for every workload")
		}
		sourceHash, err := providerSpecHash(d.Spec)
		if err != nil {
			return p, err
		}
		targetHash, err := providerSpecHash(spec)
		if err != nil {
			return p, err
		}
		metadata, err := providerMetadataHash(d)
		if err != nil {
			return p, err
		}
		// The first Helm upgrade after bootstrap replaces its creation marker
		// with the completed CA revision. Pin that exact metadata separately;
		// do not relax ownership or accept arbitrary annotation changes.
		recorded := d.DeepCopy()
		delete(recorded.Annotations, aggregationBootstrapOwner)
		recorded.Annotations[rotationRevision] = p.Aggregation.Revision
		helmMetadata, err := providerMetadataHash(recorded)
		if err != nil {
			return p, err
		}
		p.Workloads = append(p.Workloads, providerTransitionWorkload{
			Name: name, UID: d.UID, ResourceVersion: d.ResourceVersion, Replicas: *d.Spec.Replicas,
			SourceSHA256: sourceHash, TargetSHA256: targetHash, MetadataSHA256: metadata,
			HelmMetadataSHA256: helmMetadata,
			PreviousMarker:     d.Annotations[providerTransitionMarker],
		})
		for _, pod := range []corev1.PodSpec{d.Spec.Template.Spec, spec.Template.Spec} {
			for _, name := range providerConfigurationNames(pod) {
				if configurations[name] {
					continue
				}
				pin, err := s.configuration(ctx, namespace, name)
				if err != nil {
					return p, err
				}
				p.Configurations = append(p.Configurations, pin)
				configurations[name] = true
			}
		}
	}
	previous, state, err := s.read(ctx, namespace)
	if err == nil {
		if state.Phase != providerComplete || state.Plan.Target != source {
			return p, errors.New("resume the existing provider transition before planning another")
		}
		p.PreviousJournalUID, p.PreviousJournalVersion = previous.UID, previous.ResourceVersion
	} else if !apierrors.IsNotFound(err) {
		return p, err
	}
	err = s.validateTargets(ctx, providerTransitionState{Version: 1, Plan: p, Phase: providerStopping}, targets)
	if err != nil {
		return p, err
	}
	return p, s.checkInitial(ctx, p)
}

// begin requires the caller to have disclosed the outage and obtained explicit
// consent. It never changes workloads. Preflight failure performs no writes.
func (s *providerTransitionStore) begin(ctx context.Context, plan providerTransitionPlan, targets map[string]appsv1.DeploymentSpec, outageConfirmed bool) error {
	if !outageConfirmed {
		return errors.New("provider transition interrupts service and requires explicit outage confirmation")
	}
	state := providerTransitionState{Version: 1, Plan: plan, Phase: providerStopping}
	if err := s.validateTargets(ctx, state, targets); err != nil {
		return err
	}
	if err := s.checkInitial(ctx, plan); err != nil {
		return err
	}
	if _, err := s.predecessor(ctx, plan); err != nil {
		return err
	}
	return s.withLock(ctx, plan.Namespace, func(lease *coordinationv1.Lease) error {
		if err := s.validateTargets(ctx, state, targets); err != nil {
			return err
		}
		if err := s.checkInitial(ctx, plan); err != nil {
			return err
		}
		previous, err := s.predecessor(ctx, plan)
		if err != nil {
			return err
		}
		data, err := json.Marshal(state)
		if err != nil {
			return err
		}
		if err := s.checkLock(ctx, lease); err != nil {
			return err
		}
		client := s.kube.CoreV1().ConfigMaps(plan.Namespace)
		if previous != nil {
			previous.Data = map[string]string{providerTransitionKey: string(data)}
			_, err = client.Update(ctx, previous, metav1.UpdateOptions{FieldManager: FieldManager})
			return err
		}
		_, err = client.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: providerTransitionJournal, Namespace: plan.Namespace,
				Labels: map[string]string{providerTransitionMarker: plan.Release}},
			Data: map[string]string{providerTransitionKey: string(data)},
		}, metav1.CreateOptions{FieldManager: FieldManager})
		return err
	})
}

func providerMarker(plan providerTransitionPlan, phase providerTransitionPhase) string {
	return plan.ID + "/" + string(phase)
}

// inspect checks the entire fleet before touching a single workload, including
// the allowed post-write state of an interrupted attempt at this exact phase.
func (s *providerTransitionStore) inspect(ctx context.Context, state providerTransitionState) error {
	for _, w := range state.Plan.Workloads {
		d, hash, err := s.deployment(ctx, state.Plan, w)
		if err != nil {
			return err
		}
		marker, replicas := d.Annotations[providerTransitionMarker], *d.Spec.Replicas
		stopped := marker == providerMarker(state.Plan, providerStopping) && replicas == 0 && hash == w.SourceSHA256
		staged := marker == providerMarker(state.Plan, providerStaging) && replicas == 0 && hash == w.TargetSHA256
		started := marker == providerMarker(state.Plan, providerStarting) && replicas == w.Replicas && hash == w.TargetSHA256
		valid := false
		switch state.Phase {
		case providerStopping:
			valid = stopped || (marker == w.PreviousMarker && replicas == w.Replicas && hash == w.SourceSHA256)
		case providerStaging:
			valid = stopped || staged
		case providerStarting:
			valid = staged || started
		case providerVerifying, providerRecording, providerComplete:
			valid = started
		}
		if !valid {
			return fmt.Errorf("provider transition workload %s drifted from phase %s", w.Name, state.Phase)
		}
	}
	return nil
}

func (s *providerTransitionStore) drained(ctx context.Context, state providerTransitionState) error {
	pods, err := s.kube.CoreV1().Pods(state.Plan.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	sets, err := s.kube.AppsV1().ReplicaSets(state.Plan.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, w := range state.Plan.Workloads {
		d, _, err := s.deployment(ctx, state.Plan, w)
		if err != nil {
			return err
		}
		if *d.Spec.Replicas != 0 || d.Status.ObservedGeneration < d.Generation || d.Status.Replicas != 0 {
			return errProviderTransitionWaiting
		}
		selector, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
		if err != nil || selector.Empty() {
			return errors.New("provider transition requires a nonempty workload selector")
		}
		ownedSets := map[string]bool{}
		for _, rs := range sets.Items {
			owner := metav1.GetControllerOf(&rs)
			if owner == nil || owner.UID != d.UID {
				continue
			}
			ownedSets[string(rs.UID)] = true
			if ptr.Deref(rs.Spec.Replicas, 1) != 0 || rs.Status.Replicas != 0 ||
				rs.Status.ObservedGeneration < rs.Generation {
				return errProviderTransitionWaiting
			}
		}
		for _, pod := range pods.Items {
			owner := metav1.GetControllerOf(&pod)
			// Include terminating pods, selector matches without ownership, and
			// owned pods whose labels have drifted. Do not equate unready with gone.
			if selector.Matches(labels.Set(pod.Labels)) ||
				owner != nil && (owner.UID == d.UID || ownedSets[string(owner.UID)]) {
				return errProviderTransitionWaiting
			}
		}
	}
	return nil
}

func (s *providerTransitionStore) checkTargetPopulation(ctx context.Context, state providerTransitionState) error {
	sets, err := s.kube.AppsV1().ReplicaSets(state.Plan.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	pods, err := s.kube.CoreV1().Pods(state.Plan.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, w := range state.Plan.Workloads {
		d, hash, err := s.deployment(ctx, state.Plan, w)
		if err != nil {
			return err
		}
		if hash != w.TargetSHA256 {
			return errors.New("provider transition has not staged every target workload")
		}
		selector, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
		if err != nil || selector.Empty() {
			return errors.New("provider transition requires a nonempty workload selector")
		}
		ownedSets := map[types.UID]bool{}
		for _, rs := range sets.Items {
			owner := metav1.GetControllerOf(&rs)
			if owner == nil || owner.UID != w.UID {
				continue
			}
			template := rs.Spec.Template.DeepCopy()
			delete(template.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
			target := d.Spec.Template.DeepCopy()
			delete(target.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
			isTarget := apiequality.Semantic.DeepEqual(template, target)
			ownedSets[rs.UID] = isTarget
			if !isTarget && (ptr.Deref(rs.Spec.Replicas, 1) != 0 || rs.Status.Replicas != 0) {
				return errors.New("old provider ReplicaSet reappeared during transition")
			}
		}
		for _, pod := range pods.Items {
			owner := metav1.GetControllerOf(&pod)
			owned := false
			if owner != nil {
				_, owned = ownedSets[owner.UID]
			}
			if !selector.Matches(labels.Set(pod.Labels)) && !owned && (owner == nil || owner.UID != w.UID) {
				continue
			}
			if owner == nil || !ownedSets[owner.UID] {
				return errors.New("old or unowned provider pod remains during transition")
			}
		}
	}
	return nil
}

func (s *providerTransitionStore) checkReady(ctx context.Context, state providerTransitionState) error {
	if err := s.checkTargetPopulation(ctx, state); err != nil {
		return err
	}
	for _, w := range state.Plan.Workloads {
		d, _, err := s.deployment(ctx, state.Plan, w)
		if err != nil {
			return err
		}
		if d.Status.ObservedGeneration < d.Generation || d.Status.Replicas != w.Replicas ||
			d.Status.UpdatedReplicas != w.Replicas || d.Status.AvailableReplicas != w.Replicas ||
			d.Status.ReadyReplicas != w.Replicas {
			return errProviderTransitionWaiting
		}
	}
	return nil
}

func (s *providerTransitionStore) updateWorkload(ctx context.Context, lease *coordinationv1.Lease, cm *corev1.ConfigMap, state providerTransitionState, w providerTransitionWorkload, targets map[string]appsv1.DeploymentSpec) error {
	d, _, err := s.deployment(ctx, state.Plan, w)
	if err != nil {
		return err
	}
	// Read the destination object before the phase fence. A delayed write then
	// carries the older resourceVersion and cannot undo a later workload update.
	live, current, err := s.read(ctx, state.Plan.Namespace)
	if err != nil {
		return err
	}
	if live.UID != cm.UID || live.ResourceVersion != cm.ResourceVersion || current.Plan.ID != state.Plan.ID || current.Phase != state.Phase {
		return errors.New("provider transition journal changed before workload write")
	}
	if err := s.checkAggregation(ctx, state.Plan); err != nil {
		return err
	}
	if err := s.inspect(ctx, state); err != nil {
		return err
	}
	if err := s.checkAutoscalers(ctx, state.Plan); err != nil {
		return err
	}
	if err := s.checkStateConfigurations(ctx, state); err != nil {
		return err
	}
	switch state.Phase {
	case providerStopping:
		d.Spec.Replicas = ptr.To(int32(0))
	case providerStaging:
		if err := s.drained(ctx, state); err != nil {
			return err
		}
		spec := targets[w.Name]
		d.Spec = *spec.DeepCopy()
		d.Spec.Replicas = ptr.To(int32(0))
	case providerStarting:
		if err := s.checkTargetPopulation(ctx, state); err != nil {
			return err
		}
		d.Spec.Replicas = ptr.To(w.Replicas)
	default:
		return errors.New("provider transition phase cannot write workloads")
	}
	marker := providerMarker(state.Plan, state.Phase)
	if d.Annotations[providerTransitionMarker] == marker {
		return nil
	}
	if d.Annotations == nil {
		d.Annotations = map[string]string{}
	}
	d.Annotations[providerTransitionMarker] = marker
	if err := s.checkLock(ctx, lease); err != nil {
		return err
	}
	_, err = s.kube.AppsV1().Deployments(state.Plan.Namespace).Update(ctx, d, metav1.UpdateOptions{FieldManager: FieldManager})
	return err
}

// step performs one durable phase. Cancellation and failed convergence leave
// intent intact; retries move forward, never restore an old protocol automatically.
func (s *providerTransitionStore) step(ctx context.Context, namespace, transaction string, targets map[string]appsv1.DeploymentSpec) error {
	before, planned, err := s.read(ctx, namespace)
	if err != nil {
		return err
	}
	if planned.Plan.ID != transaction {
		return errors.New("provider transition transaction changed")
	}
	if err := s.validateTargets(ctx, planned, targets); err != nil {
		return err
	}
	if err := s.inspect(ctx, planned); err != nil {
		return err
	}
	return s.withLock(ctx, namespace, func(lease *coordinationv1.Lease) error {
		cm, state, err := s.read(ctx, namespace)
		if err != nil {
			return err
		}
		if cm.UID != before.UID || cm.ResourceVersion != before.ResourceVersion || state.Plan.ID != transaction {
			return errors.New("provider transition transaction changed")
		}
		if err := s.validateTargets(ctx, state, targets); err != nil {
			return err
		}
		if err := s.inspect(ctx, state); err != nil {
			return err
		}
		switch state.Phase {
		case providerStopping, providerStaging, providerStarting:
			for _, w := range state.Plan.Workloads {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := s.updateWorkload(ctx, lease, cm, state, w, targets); err != nil {
					return err
				}
			}
			if state.Phase != providerStarting {
				if err := s.drained(ctx, state); err != nil {
					return err
				}
			}
			next := map[providerTransitionPhase]providerTransitionPhase{
				providerStopping: providerStaging, providerStaging: providerStarting, providerStarting: providerVerifying,
			}[state.Phase]
			return s.advance(ctx, lease, cm, state, next)
		case providerVerifying, providerComplete:
			if err := s.checkReady(ctx, state); err != nil {
				return err
			}
			if err := s.checks.verify(ctx, state.Plan); err != nil {
				return err
			}
			if err := s.inspect(ctx, state); err != nil {
				return err
			}
			if err := s.checkReady(ctx, state); err != nil {
				return err
			}
			if err := s.checkConfigurations(ctx, state.Plan); err != nil {
				return err
			}
			if err := s.checkAutoscalers(ctx, state.Plan); err != nil {
				return err
			}
			if err := s.checkAggregation(ctx, state.Plan); err != nil {
				return err
			}
			if err := s.checkLock(ctx, lease); err != nil {
				return err
			}
			if state.Phase == providerComplete {
				return nil
			}
			next := providerComplete
			if state.Plan.Helm != nil {
				next = providerRecording
			}
			return s.advance(ctx, lease, cm, state, next)
		case providerRecording:
			return errors.New("provider transition requires its pinned final Helm recording")
		default:
			return errors.New("unknown provider transition phase")
		}
	})
}

// record keeps the journal unfinished while Helm catches up with the verified
// workloads. The callback must record and verify the pinned release, using the
// supplied fence before writes. A failed or interrupted recording stays resumable.
func (s *providerTransitionStore) record(ctx context.Context, namespace, transaction string,
	targets map[string]appsv1.DeploymentSpec, record func(context.Context, func(context.Context) error) error) error {
	if record == nil {
		return errors.New("provider transition requires a final Helm recorder")
	}
	before, planned, err := s.read(ctx, namespace)
	if err != nil {
		return err
	}
	if planned.Plan.ID != transaction || planned.Phase != providerRecording || planned.Plan.Helm == nil {
		return errors.New("provider transition is not awaiting its pinned Helm recording")
	}
	return s.withLock(ctx, namespace, func(lease *coordinationv1.Lease) error {
		fence := func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			cm, current, err := s.read(ctx, namespace)
			if err != nil {
				return err
			}
			if cm.UID != before.UID || cm.ResourceVersion != before.ResourceVersion ||
				!rotationJSONEqual(current, planned) {
				return errors.New("provider transition changed during Helm recording")
			}
			if err := s.validateTargets(ctx, planned, targets); err != nil {
				return err
			}
			if err := s.inspect(ctx, planned); err != nil {
				return err
			}
			if err := s.checkReady(ctx, planned); err != nil {
				return err
			}
			return s.checkLock(ctx, lease)
		}
		if err := fence(ctx); err != nil {
			return err
		}
		if err := s.checks.verify(ctx, planned.Plan); err != nil {
			return err
		}
		if err := fence(ctx); err != nil {
			return err
		}
		if err := record(ctx, fence); err != nil {
			return err
		}
		if err := fence(ctx); err != nil {
			return err
		}
		if err := s.checks.verify(ctx, planned.Plan); err != nil {
			return err
		}
		if err := fence(ctx); err != nil {
			return err
		}
		return s.advance(ctx, lease, before, planned, providerComplete)
	})
}
