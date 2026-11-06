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
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/radius-project/radius/pkg/ucp/frontend/versions"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
)

const (
	rotationRoute   = "radius.io/aggregation-route"
	rotationOldHash = "radius.io/aggregation-old-hash"
	rotationNewHash = "radius.io/aggregation-new-hash"
)

// observeAggregatedPod performs a real resource read, not cached discovery. Its
// response must identify an independently observed new UCP pod. It verifies the
// configured Kubernetes endpoint, not unseen HA API-server transports.
func observeAggregatedPod(ctx context.Context, config *rest.Config, expected map[types.UID]bool) error {
	if config == nil || len(expected) == 0 {
		return errors.New("aggregation observation requires an authenticated client and expected UCP pods")
	}
	config = rest.CopyConfig(config)
	config.Timeout = 10 * time.Second
	client, err := rest.HTTPClientFor(config)
	if err != nil {
		return fmt.Errorf("cannot create aggregation verification client: %w", err)
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	endpoint, err := url.Parse(strings.TrimRight(config.Host, "/") + "/apis/api.ucp.dev/v1alpha3/planes/radius/local")
	if err != nil {
		return errors.New("invalid Kubernetes endpoint for aggregation verification")
	}
	query := endpoint.Query()
	query.Set("api-version", versions.DefaultAPIVersion)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	request.Close = true
	request.Header.Set("Cache-Control", "no-cache")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("aggregated Radius read failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("aggregated Radius read returned HTTP %d", response.StatusCode)
	}
	if !expected[types.UID(response.Header.Get(mtls.AggregationPodUIDHeader))] {
		return errors.New("aggregated Radius read did not reach an expected new UCP process")
	}
	_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
	return err
}

func rotationServingSecret(spec corev1.PodSpec) string {
	for _, volume := range spec.Volumes {
		if volume.Name != "cert" {
			continue
		}
		if volume.Secret != nil {
			return volume.Secret.SecretName
		}
		if volume.Projected != nil {
			for _, source := range volume.Projected.Sources {
				if source.Secret != nil {
					return source.Secret.Name
				}
			}
		}
	}
	return ""
}

func (s *rotationStore) aggregationPods(ctx context.Context, state rotationState, candidate bool) (map[types.UID]bool, string, error) {
	var ucp rotationWorkload
	for _, workload := range state.Plan.Consumers {
		if workload.Name == "ucp" {
			ucp = workload
		}
	}
	pods, err := s.kube.CoreV1().Pods(s.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=ucp", Limit: 1000,
	})
	if err != nil {
		return nil, "", fmt.Errorf("cannot inspect aggregation backend pods: %w", err)
	}
	if pods.Continue != "" {
		return nil, "", errors.New("aggregation backend inventory exceeds its supported bound")
	}
	ids := map[types.UID]bool{}
	hash := ""
	for _, pod := range pods.Items {
		if (rotationServingSecret(pod.Spec) == state.Plan.ServingSecret) != candidate {
			continue
		}
		owner := metav1.GetControllerOf(&pod)
		if owner == nil || owner.APIVersion != "apps/v1" || owner.Kind != "ReplicaSet" || pod.DeletionTimestamp != nil {
			continue
		}
		set, err := s.kube.AppsV1().ReplicaSets(s.namespace).Get(ctx, owner.Name, metav1.GetOptions{})
		if err != nil {
			return nil, "", err
		}
		parent := metav1.GetControllerOf(set)
		if set.UID != owner.UID || parent == nil || parent.UID != ucp.UID || parent.Name != "ucp" || parent.Kind != "Deployment" {
			return nil, "", errors.New("aggregation backend pod has unexpected ownership")
		}
		target := TLSProofTarget{PodUID: pod.UID, ContainerName: "ucp"}
		status, err := proofContainer(&pod, target)
		if err != nil || !status.Ready {
			continue
		}
		podHash := pod.Labels[appsv1.DefaultDeploymentUniqueLabelKey]
		if podHash == "" || hash != "" && hash != podHash {
			return nil, "", errors.New("aggregation backend has ambiguous rollout generations")
		}
		hash = podHash
		ids[pod.UID] = true
	}
	if len(ids) == 0 {
		return nil, "", errors.New("no ready aggregation backend for the requested serving generation")
	}
	return ids, hash, nil
}

func (s *rotationStore) setAggregationRoute(ctx context.Context, transaction, route, hash string, observed *corev1.Service) (*corev1.Service, error) {
	services := s.kube.CoreV1().Services(s.namespace)
	var service *corev1.Service
	if observed != nil {
		service = observed.DeepCopy()
	} else {
		var err error
		service, err = services.Get(ctx, "ucp", metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
	}
	state, err := s.plannedPhase(ctx, transaction, rotationIssue)
	if err != nil {
		return nil, err
	}
	if service.Name != "ucp" || service.Namespace != s.namespace || service.UID != state.Plan.ServiceUID ||
		!rotationOwned(service, s.namespace, s.release) {
		return nil, errors.New("aggregation Service identity or ownership changed")
	}
	if err := checkRotationObjectRevision(service, state); err != nil {
		return nil, err
	}
	annotations := service.Annotations
	currentRoute := ""
	if annotations[rotationTransaction] == transaction {
		currentRoute = annotations[rotationRoute]
	} else {
		delete(annotations, rotationOldHash)
		delete(annotations, rotationNewHash)
	}
	if currentRoute == "complete" && route != "complete" {
		return nil, errors.New("aggregation migration already completed; refusing to restore an earlier route")
	}
	selector := maps.Clone(state.Plan.ServiceSelector)
	valid := maps.Equal(service.Spec.Selector, selector)
	if annotations[rotationTransaction] == transaction {
		for _, key := range []string{rotationOldHash, rotationNewHash} {
			if annotations[key] != "" {
				prior := maps.Clone(selector)
				prior[appsv1.DefaultDeploymentUniqueLabelKey] = annotations[key]
				valid = valid || maps.Equal(service.Spec.Selector, prior)
			}
		}
	}
	if !valid {
		return nil, errors.New("aggregation Service selector changed outside the rotation")
	}
	switch route {
	case "old":
		if hash == "" || currentRoute == "new" && observed == nil ||
			currentRoute != "" && hash != annotations[rotationOldHash] {
			return nil, errors.New("old aggregation routing requires its original generation and the exact failed-probe Service version")
		}
		selector[appsv1.DefaultDeploymentUniqueLabelKey] = hash
		annotations[rotationOldHash] = hash
	case "new":
		if hash == "" || annotations[rotationTransaction] != transaction || annotations[rotationOldHash] == "" {
			return nil, errors.New("new aggregation routing requires a retained old backend")
		}
		if currentRoute != "old" && currentRoute != "new" || annotations[rotationNewHash] != "" && annotations[rotationNewHash] != hash {
			return nil, errors.New("aggregation candidate generation or route changed")
		}
		selector[appsv1.DefaultDeploymentUniqueLabelKey] = hash
		annotations[rotationNewHash] = hash
	case "complete":
		if currentRoute == "complete" {
			return service, nil
		}
		if currentRoute != "new" || annotations[rotationNewHash] == "" || observed == nil {
			return nil, errors.New("aggregation routing requires the exact successfully probed candidate Service version")
		}
	default:
		return nil, errors.New("invalid aggregation route transition")
	}
	if currentRoute == route && maps.Equal(service.Spec.Selector, selector) {
		return service, nil
	}
	annotations[rotationTransaction], annotations[rotationRevision], annotations[rotationRoute] = transaction, state.revision(), route
	service.Spec.Selector, service.Annotations = selector, annotations
	updated, err := services.Update(ctx, service, metav1.UpdateOptions{FieldManager: FieldManager})
	if err != nil {
		return nil, fmt.Errorf("cannot change aggregation routing; reread before retrying: %w", err)
	}
	return updated, nil
}
