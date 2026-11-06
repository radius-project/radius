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
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
)

type rotationRunner struct {
	store   *rotationStore
	config  *rest.Config
	log     func(string, ...any)
	observe func(context.Context, *rest.Config, TLSProofTarget) (PodTLSProof, error)
	probe   func(context.Context, *rest.Config, map[types.UID]bool) error
}

func (r *rotationRunner) run(ctx context.Context, transaction string, requested rotationPhase) error {
	if !slices.Contains([]rotationPhase{rotationPublish, rotationAdopt, rotationIssue, rotationRetire, rotationRecover}, requested) {
		return errors.New("choose CA rotation phase publish, adopt, issue, retire, or recover")
	}
	journal, state, err := r.store.read(ctx)
	if err != nil {
		return err
	}
	if state.ID != transaction || state.Plan == nil {
		return errors.New("rotation transaction or resource plan changed")
	}
	if requested == rotationRecover {
		if !state.Recovery && state.Phase != rotationComplete {
			return errors.New("incident recovery must first record its explicit new-only intent")
		}
		requested = rotationRetire
	}
	if state.Phase == rotationComplete {
		return r.store.reconcileTrust(ctx, transaction)
	}
	// Recover a lost response between advancing the journal and stamping public
	// trust. Retirement is different: its consumer barrier must run first.
	if state.Phase != rotationRetire {
		if err := r.store.reconcileTrust(ctx, transaction); err != nil {
			return err
		}
	}
	if requested == rotationRetire && state.Phase == rotationReady {
		// Retirement is never an automatic consequence of successful issuance.
		// Re-observe the live fleet and aggregation before recording this intent.
		if err := r.verifyConsumers(ctx, state); err != nil {
			return err
		}
		if err := r.verifyAggregation(ctx, state); err != nil {
			return err
		}
		if err := r.store.advance(ctx, journal, state, rotationRetire); err != nil {
			return err
		}
		journal, state, err = r.store.read(ctx)
		if err != nil {
			return err
		}
	}
	if slices.Index(rotationPhases, state.Phase) > slices.Index(rotationPhases, requested) {
		return nil
	}
	if state.Phase != requested {
		return fmt.Errorf("CA rotation is in phase %s; complete it before %s", state.Phase, requested)
	}
	if err := r.store.checkPlannedSigner(ctx, state); err != nil {
		return err
	}
	var next rotationPhase
	switch requested {
	case rotationPublish:
		if _, _, err := r.store.publishComponentTrust(ctx, rotationComponentBundle(state)); err != nil {
			return err
		}
		if err := r.store.reconcileTrust(ctx, transaction); err != nil {
			return err
		}
		next = rotationAdopt
	case rotationAdopt:
		for _, workload := range state.Plan.Consumers {
			if err := r.store.reconcileRotationDeployment(ctx, transaction, workload); err != nil {
				return err
			}
		}
		if err := r.wait(ctx, "component trust adoption", func() error { return r.verifyConsumers(ctx, state) }); err != nil {
			return err
		}
		next = rotationIssue
	case rotationIssue:
		if err := r.issueLeaves(ctx, state); err != nil {
			return err
		}
		for _, workload := range state.Plan.Consumers {
			if workload.Name != "ucp" {
				if err := r.store.reconcileRotationDeployment(ctx, transaction, workload); err != nil {
					return err
				}
			}
		}
		if err := r.migrateAggregation(ctx, state); err != nil {
			return err
		}
		if err := r.wait(ctx, "issued leaf adoption", func() error { return r.verifyConsumers(ctx, state) }); err != nil {
			return err
		}
		if err := r.verifyAggregation(ctx, state); err != nil {
			return err
		}
		next = rotationReady
	case rotationRetire:
		if state.Recovery {
			// Emergency recovery never republishes overlap or rolls trust back.
			// Failures leave explicit new-only intent for another bounded retry.
			if err := r.store.reconcileTrust(ctx, transaction); err != nil {
				return err
			}
			if err := r.issueLeaves(ctx, state); err != nil {
				return err
			}
			if err := r.store.reconcileRecoveryRoute(ctx, transaction); err != nil {
				return err
			}
		}
		for _, workload := range state.Plan.Consumers {
			if err := r.store.reconcileRotationDeployment(ctx, transaction, workload); err != nil {
				return err
			}
		}
		if err := r.wait(ctx, "old component trust retirement", func() error { return r.verifyConsumers(ctx, state) }); err != nil {
			return err
		}
		if err := r.store.reconcileTrust(ctx, transaction); err != nil {
			return err
		}
		if err := r.wait(ctx, "aggregation trust retirement", func() error { return r.verifyAggregation(ctx, state) }); err != nil {
			return err
		}
		next = rotationComplete
	}
	if err := r.store.advance(ctx, journal, state, next); err != nil {
		return err
	}
	return r.store.reconcileTrust(ctx, transaction)
}

func (r *rotationRunner) issueLeaves(ctx context.Context, state rotationState) error {
	if err := r.store.ensureRotationIssuer(ctx, state.ID); err != nil {
		return err
	}
	for _, workload := range state.Plan.Consumers {
		if err := r.store.switchRotationCertificate(ctx, state.ID, workload); err != nil {
			return err
		}
	}
	if err := r.store.ensureAggregationCertificate(ctx, state.ID); err != nil {
		return err
	}
	return r.wait(ctx, "cert-manager issuance", func() error {
		if _, err := r.store.rotationConsumerTargets(ctx, state); err != nil {
			return err
		}
		serving, err := r.store.kube.CoreV1().Secrets(r.store.namespace).Get(ctx, state.Plan.ServingSecret, metav1.GetOptions{})
		if err != nil {
			return err
		}
		_, err = rotationLeafDigest(serving, state.NewCA, r.store.namespace, "ucp", false, r.store.now())
		return err
	})
}

func (r *rotationRunner) verifyConsumers(ctx context.Context, state rotationState) error {
	targets, err := r.store.rotationConsumerTargets(ctx, state)
	if err != nil {
		return err
	}
	observer := rotationConsumerObserver{kube: r.store.kube, config: r.config,
		namespace: r.store.namespace, release: r.store.release, observe: r.observe}
	return observer.verify(ctx, targets)
}

func (r *rotationRunner) verifyAggregation(ctx context.Context, state rotationState) error {
	api, err := r.store.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if err := r.store.checkAPIService(api); err != nil {
		return err
	}
	bundle, err := apiServiceBundle(api)
	if err != nil {
		return err
	}
	if api.GetUID() != state.APIServiceUID || api.GetAnnotations()[rotationRevision] != state.revision() || !bytes.Equal(bundle, state.bundle()) {
		return errors.New("aggregation APIService identity, phase, or public trust changed")
	}
	service, err := r.store.kube.CoreV1().Services(r.store.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if service.UID != state.Plan.ServiceUID || !rotationOwned(service, r.store.namespace, r.store.release) ||
		service.Annotations[rotationTransaction] != state.ID || service.Annotations[rotationRoute] != "complete" ||
		!maps.Equal(service.Spec.Selector, state.Plan.ServiceSelector) {
		return errors.New("aggregation Service has not completed its owned routing transition")
	}
	conditions, _, err := unstructured.NestedSlice(api.Object, "status", "conditions")
	if err != nil {
		return err
	}
	available := false
	for _, condition := range conditions {
		value, ok := condition.(map[string]any)
		if ok && value["type"] == "Available" && value["status"] == "True" {
			available = true
		}
	}
	if !available {
		return errors.New("Radius APIService is not available")
	}
	ids, _, err := r.store.aggregationPods(ctx, state, true)
	if err != nil {
		return err
	}
	probe := r.probe
	if probe == nil {
		probe = observeAggregatedPod
	}
	if err := probe(ctx, r.config, ids); err != nil {
		return err
	}
	after, err := r.store.kube.CoreV1().Services(r.store.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if after.UID != service.UID || after.ResourceVersion != service.ResourceVersion {
		return errors.New("aggregation Service changed during observation")
	}
	return nil
}

func (r *rotationRunner) observeAggregationCandidates(ctx context.Context, state rotationState) (map[types.UID]bool, string, error) {
	ids, hash, err := r.store.aggregationPods(ctx, state, true)
	if err != nil {
		return nil, "", err
	}
	targets, err := r.store.rotationConsumerTargets(ctx, state)
	if err != nil {
		return nil, "", err
	}
	var expected rotationConsumer
	for _, target := range targets {
		if target.Name == "ucp" {
			expected = target
		}
	}
	pods, err := r.store.kube.CoreV1().Pods(r.store.namespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=ucp", Limit: 1000})
	if err != nil {
		return nil, "", err
	}
	if pods.Continue != "" {
		return nil, "", errors.New("aggregation candidate inventory exceeds its supported bound")
	}
	observe := r.observe
	if observe == nil {
		observe = ObserveTLSStartupProof
	}
	observed := map[types.UID]bool{}
	for _, pod := range pods.Items {
		if !ids[pod.UID] {
			continue
		}
		if err := rotationConsumerTemplate(corev1.PodTemplateSpec{Spec: pod.Spec}, expected); err != nil {
			return nil, "", err
		}
		servingProjection := rotationTLSProjection("cert", state.Plan.ServingSecret, state.Plan.TargetTrustMap)
		servingMounted, servingProjected := false, false
		for _, volume := range pod.Spec.Volumes {
			if volume.Name == "cert" && volume.Projected != nil &&
				apiequality.Semantic.DeepEqual(volume.Projected.Sources, servingProjection.Projected.Sources) {
				servingProjected = true
			}
		}
		for _, container := range pod.Spec.Containers {
			if container.Name == "ucp" {
				for _, mount := range container.VolumeMounts {
					if mount.Name == "cert" && mount.MountPath == "/var/tls/cert" && mount.ReadOnly && mount.SubPath == "" && mount.SubPathExpr == "" {
						servingMounted = true
					}
				}
			}
		}
		if !servingMounted || !servingProjected {
			return nil, "", errors.New("aggregation candidate does not mount the intended dedicated serving identity and trust")
		}
		target := TLSProofTarget{Namespace: r.store.namespace, PodName: pod.Name, PodUID: pod.UID, ContainerName: "ucp",
			Port: expected.ProofPort, TrustBundleSHA256: expected.TrustSHA256, LeafSHA256: expected.LeafSHA256}
		proof, err := observe(ctx, r.config, target)
		if err != nil {
			return nil, "", err
		}
		if err := proof.Evidence.Validate(string(pod.UID), target.TrustBundleSHA256, target.LeafSHA256, time.Now()); err != nil {
			return nil, "", err
		}
		observed[pod.UID] = true
	}
	if !maps.Equal(ids, observed) {
		return nil, "", errors.New("aggregation candidate inventory changed before process observation")
	}
	after, afterHash, err := r.store.aggregationPods(ctx, state, true)
	if err != nil || afterHash != hash || !maps.Equal(ids, after) {
		return nil, "", errors.New("aggregation candidate inventory changed during process observation")
	}
	return ids, hash, nil
}

func (r *rotationRunner) wait(ctx context.Context, operation string, check func() error) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	lastMessage := ""
	for {
		err := check()
		if err == nil {
			return nil
		}
		if r.log != nil && err.Error() != lastMessage {
			r.log("Waiting for %s: %v", operation, err)
			lastMessage = err.Error()
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s interrupted; the recorded intent and resources are retained for retry: %w", operation, errors.Join(ctx.Err(), err))
		case <-ticker.C:
		}
	}
}

func (r *rotationRunner) migrateAggregation(ctx context.Context, state rotationState) error {
	var ucp rotationWorkload
	for _, workload := range state.Plan.Consumers {
		if workload.Name == "ucp" {
			ucp = workload
		}
	}
	service, err := r.store.kube.CoreV1().Services(r.store.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return err
	}
	completed := service.Annotations[rotationTransaction] == state.ID && service.Annotations[rotationRoute] == "complete"
	if !completed {
		if service.Annotations[rotationTransaction] != state.ID {
			_, oldHash, err := r.store.aggregationPods(ctx, state, false)
			if err != nil {
				return err
			}
			if _, err := r.store.setAggregationRoute(ctx, state.ID, "old", oldHash, nil); err != nil {
				return err
			}
		}
		if err := r.store.reconcileRotationDeployment(ctx, state.ID, ucp); err != nil {
			return err
		}
		var ids map[types.UID]bool
		var hash string
		if err := r.wait(ctx, "new aggregation backend", func() error {
			var err error
			ids, hash, err = r.observeAggregationCandidates(ctx, state)
			return err
		}); err != nil {
			return err
		}
		routed, err := r.store.setAggregationRoute(ctx, state.ID, "new", hash, nil)
		if err != nil {
			return err
		}
		probe := r.probe
		if probe == nil {
			probe = observeAggregatedPod
		}
		if err := r.wait(ctx, "new aggregated serving identity", func() error {
			current, currentHash, err := r.observeAggregationCandidates(ctx, state)
			if err != nil {
				return err
			}
			if hash != currentHash {
				return errors.New("aggregation candidate generation changed; retry with a fresh route")
			}
			ids = current
			if err := probe(ctx, r.config, ids); err != nil {
				return err
			}
			current, currentHash, err = r.store.aggregationPods(ctx, state, true)
			if err != nil {
				return err
			}
			if hash != currentHash || !maps.Equal(ids, current) {
				return errors.New("aggregation candidate inventory changed; retry with a fresh observation")
			}
			return nil
		}); err != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			_, restoreErr := r.store.setAggregationRoute(cleanup, state.ID, "old", routed.Annotations[rotationOldHash], routed)
			return errors.Join(err, restoreErr)
		}
		if _, err := r.store.setAggregationRoute(ctx, state.ID, "complete", "", routed); err != nil {
			return err
		}
	} else if err := r.verifyAggregation(ctx, state); err != nil {
		return err
	}
	return r.store.reconcileRotationDeployment(ctx, state.ID, ucp)
}
