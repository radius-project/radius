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
	"maps"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func (s *rotationStore) bindBootstrapLifecycle(ctx context.Context, state aggregationBootstrapState) (rotationState, error) {
	plan, err := state.resourcePlan()
	if err != nil {
		return rotationState{}, err
	}
	get := func(resource schema.GroupVersionResource, name string) (*unstructured.Unstructured, error) {
		namespace := s.namespace
		if resource == apiServiceResource {
			namespace = ""
		}
		object, err := s.dynamic.Resource(resource).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if err := s.recordBootstrapResource(ctx, state.ID, resource, object); err != nil {
			return nil, err
		}
		return object, nil
	}
	api, err := get(apiServiceResource, radiusAPIService)
	if err != nil {
		return rotationState{}, err
	}
	if err := s.checkAPIService(api); err != nil {
		return rotationState{}, err
	}
	bundle, err := apiServiceBundle(api)
	if err != nil || !bytes.Equal(bundle, state.CA) {
		return rotationState{}, errors.New("bootstrap APIService does not use the selected public CA")
	}
	service, err := get(corev1.SchemeGroupVersion.WithResource("services"), "ucp")
	if err != nil {
		return rotationState{}, err
	}
	plan.ServiceUID = service.GetUID()
	plan.ServiceSelector, _, err = unstructured.NestedStringMap(service.Object, "spec", "selector")
	if err != nil {
		return rotationState{}, err
	}
	names := []string{"ucp"}
	if state.Scope == rotationComponentsAndAggregation {
		names = rotationComponents
	}
	for _, name := range names {
		object, err := get(appsv1.SchemeGroupVersion.WithResource("deployments"), name)
		if err != nil {
			return rotationState{}, err
		}
		var deployment appsv1.Deployment
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &deployment); err != nil {
			return rotationState{}, err
		}
		port, err := bootstrapComponentProofPort(deployment, name)
		if err != nil {
			return rotationState{}, err
		}
		if name == "ucp" {
			if deployment.Spec.ProgressDeadlineSeconds == nil || port != state.ProofPort {
				return rotationState{}, errors.New("bootstrap UCP rollout settings or proof listener changed")
			}
			plan.Aggregation = &rotationAggregationWorkload{DeploymentUID: deployment.UID, ProofPort: port}
			plan.UCPMinReadySeconds, plan.UCPProgressDeadlineSeconds = deployment.Spec.MinReadySeconds, *deployment.Spec.ProgressDeadlineSeconds
		}
		if state.Scope == rotationAggregationOnly {
			continue
		}
		certificate, err := get(rotationCertificateResource, name+"-cert")
		if err != nil {
			return rotationState{}, err
		}
		reference, _, err := unstructured.NestedMap(certificate.Object, "spec", "issuerRef")
		if err != nil || !rotationIssuerRefsEqual(reference, plan.issuerReference()) {
			return rotationState{}, errors.New("bootstrap component Certificate uses a different issuer")
		}
		issuer, err := issuerReference(map[string]any{"issuerRef": reference})
		if err != nil {
			return rotationState{}, err
		}
		plan.Consumers = append(plan.Consumers, rotationWorkload{Name: name, UID: deployment.UID, ProofPort: port,
			CertificateUID: certificate.GetUID(), OldIssuer: issuer.Name, OldIssuerGroup: issuer.Group, OldIssuerKind: issuer.Kind})
	}
	completed := rotationState{SchemaVersion: 3, Bootstrap: true, ID: state.ID, Namespace: s.namespace, Release: s.release,
		Phase: rotationComplete, CreatedAt: s.now().UTC(), APIServiceUID: api.GetUID(), NewCA: bytes.Clone(state.CA), Plan: plan}
	return completed, completed.validate(s.now())
}

func bootstrapComponentProofPort(deployment appsv1.Deployment, name string) (uint16, error) {
	if name == "ucp" {
		return aggregationProofPort(deployment.Spec.Template.Spec)
	}
	containerName, portName := name, "metrics"
	if name == "bicep-de" {
		containerName, portName = "de", "bicep-de-api"
	}
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == containerName {
			for _, port := range container.Ports {
				if port.Name == portName && port.ContainerPort > 0 && port.ContainerPort <= 65535 &&
					(port.Protocol == "" || port.Protocol == corev1.ProtocolTCP) {
					return uint16(port.ContainerPort), nil
				}
			}
		}
	}
	return 0, errors.New("bootstrap component has no fixed process-evidence listener")
}

func (r *rotationRunner) verifyBootstrapLifecycle(ctx context.Context, state rotationState) error {
	if !state.Bootstrap || state.SchemaVersion != 3 || state.Phase != rotationComplete {
		return errors.New("bootstrap verification requires the bound initial lifecycle")
	}
	if state.Plan.effectiveScope() == rotationComponentsAndAggregation {
		if err := r.verifyConsumers(ctx, state); err != nil {
			return err
		}
	}
	api, err := r.store.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if err := r.store.checkAPIService(api); err != nil {
		return err
	}
	bundle, err := apiServiceBundle(api)
	if err != nil || api.GetUID() != state.APIServiceUID || !bytes.Equal(bundle, state.NewCA) {
		return errors.New("bootstrap aggregation identity or public trust changed")
	}
	if err := requireAggregationAvailable(api); err != nil {
		return err
	}
	service, err := r.store.kube.CoreV1().Services(r.store.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if service.UID != state.Plan.ServiceUID || !rotationOwned(service, r.store.namespace, r.store.release) ||
		!maps.Equal(service.Spec.Selector, state.Plan.ServiceSelector) {
		return errors.New("bootstrap aggregation routing changed")
	}
	evidence, err := r.store.aggregationServingEvidence(ctx, r.config, state, r.observeAggregation)
	if err != nil {
		return err
	}
	ids := map[types.UID]bool{}
	for _, target := range evidence.targets {
		ids[target.PodUID] = true
	}
	probe := r.probe
	if probe == nil {
		probe = observeAggregatedPod
	}
	if err := probe(ctx, r.config, ids); err != nil {
		return err
	}
	if state.Plan.effectiveScope() == rotationComponentsAndAggregation {
		if err := r.verifyConsumers(ctx, state); err != nil {
			return err
		}
	}
	current, err := r.store.aggregationInventory(ctx, state)
	if err != nil {
		return err
	}
	if !maps.Equal(evidence.versions, current.versions) {
		return errors.New("bootstrap serving fleet changed during the API-server probe")
	}
	liveAPI, err := r.store.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if err != nil {
		return err
	}
	liveService, err := r.store.kube.CoreV1().Services(r.store.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if api.GetUID() != liveAPI.GetUID() || api.GetResourceVersion() != liveAPI.GetResourceVersion() ||
		service.UID != liveService.UID || service.ResourceVersion != liveService.ResourceVersion {
		return errors.New("bootstrap public trust or routing changed during evidence collection")
	}
	return nil
}

func (s *rotationStore) recordBootstrapLifecycle(ctx context.Context, state rotationState) error {
	if !state.Bootstrap {
		return errors.New("initial lifecycle recording requires verified bootstrap origin")
	}
	if err := state.validate(s.now()); err != nil {
		return err
	}
	_, current, err := s.read(ctx)
	if err == nil {
		if !current.Bootstrap || current.ID != state.ID || current.APIServiceUID != state.APIServiceUID ||
			!reflect.DeepEqual(current.Plan, state.Plan) || !bytes.Equal(current.NewCA, state.NewCA) {
			return errors.New("bootstrap cannot replace an existing CA lifecycle record")
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil || len(data) > maxRotationBytes {
		return errors.New("bootstrap lifecycle exceeds its schema size limit")
	}
	if err := s.checkWrite(ctx); err != nil {
		return err
	}
	_, err = s.kube.CoreV1().ConfigMaps(s.namespace).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: rotationJournalName, Namespace: s.namespace, Labels: map[string]string{rotationOwner: s.release}},
		Data:       map[string]string{rotationJournalKey: string(data)},
	}, metav1.CreateOptions{FieldManager: FieldManager})
	return err
}
