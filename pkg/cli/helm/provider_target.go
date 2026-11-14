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
	"strings"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	appsv1 "k8s.io/api/apps/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func providerTargetValues(source *releasev1.Release, target, transaction string, overrides map[string]any,
	replicas map[string]int32) (map[string]any, error) {
	id, parseErr := uuid.Parse(transaction)
	if source == nil || source.Chart == nil || parseErr != nil || id == uuid.Nil || id.String() != transaction || target != "radius" && target != "external" {
		return nil, errors.New("provider transition requires a source chart and exact target intent")
	}
	if source.Chart.Metadata == nil || source.Chart.Metadata.Annotations["radius.io/provider-transition-protocol"] != "2" {
		return nil, errors.New("provider transition requires a selected chart supporting token/CSI protocol v2")
	}
	old := valuesutil.CoalesceTables(copyValues(source.Config), copyValues(source.Chart.Values))
	mode, err := resolveAuthzMode(nil, old)
	if err != nil {
		return nil, err
	}
	if mode == authz.ModeOff {
		return nil, errors.New("provider transition requires active authorization; Off is not a transition bridge")
	}
	selected, _, err := providerValue(old)
	if err != nil {
		return nil, err
	}
	if selected == target {
		return nil, errors.New("provider transition must change the installed provider")
	}
	for _, name := range []string{"ucpMTLS", "externalIdentity"} {
		enabled, _, err := unstructured.NestedBool(old, "de", name, "enabled")
		if err != nil || enabled {
			return nil, fmt.Errorf("de.%s.enabled is a legacy contract; provider protocol v2 requires internalAuth", name)
		}
	}
	if err := validateProviderOverrides(overrides, "", target); err != nil {
		return nil, err
	}
	values := valuesutil.CoalesceTables(copyValues(overrides), old)
	optIn := "internalAuth"
	enabled, _, err := unstructured.NestedBool(values, "de", optIn, "enabled")
	if err != nil || !enabled {
		return nil, fmt.Errorf("provider transition requires explicit de.%s.enabled=true for the already installed paired engine image", optIn)
	}
	for _, field := range []string{"image", "tag"} {
		value, _, err := unstructured.NestedString(source.Config, "de", field)
		if err != nil || value == "" {
			return nil, errors.New("provider transition requires an explicit installed paired engine image and tag; change images in a separate upgrade")
		}
	}
	changes := []struct {
		path  []string
		value any
	}{
		{[]string{"global", "rbac", "tls", "provider"}, target},
		{[]string{"global", "rbac", providerGenerationValue}, strings.ReplaceAll(transaction, "-", "")},
	}
	for _, change := range changes {
		if err := unstructured.SetNestedField(values, change.value, change.path...); err != nil {
			return nil, err
		}
	}
	if len(replicas) != len(rotationComponents) {
		return nil, errors.New("provider transition requires the exact source replica inventory")
	}
	counts := map[string]any{}
	for _, component := range rotationComponents {
		count, found := replicas[component]
		if !found || count < 1 || component == "controller" && count != 1 {
			return nil, errors.New("provider readiness requires running components and exactly one controller")
		}
		counts[component] = int64(count)
	}
	if err := unstructured.SetNestedMap(values, counts, "global", "rbac", "providerReplicas"); err != nil {
		return nil, err
	}
	return values, nil
}

func validateProviderOverrides(values map[string]any, prefix, target string) error {
	allowed := map[string]bool{
		"de.internalAuth.enabled": true,
	}
	for key, value := range values {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if nested, ok := value.(map[string]any); ok {
			if len(nested) == 0 {
				return errors.New("provider transition cannot include empty or unrelated configuration overrides")
			}
			if err := validateProviderOverrides(nested, path, target); err != nil {
				return err
			}
			continue
		}
		expected, found := allowed[path]
		actual, boolean := value.(bool)
		if !found || !boolean || actual != expected {
			return errors.New("provider transition permits only the common engine capability switch; change CA, images and other settings separately")
		}
	}
	return nil
}

// Use API defaulting and admission before hashing a target. This changes neither
// the live Deployment nor its generation; actual staging remains fenced at zero.
func (s *providerTransitionStore) materializeTargets(ctx context.Context, namespace string,
	prepared providerPreparedResources, replicas map[string]int32) (map[string]appsv1.DeploymentSpec, error) {
	targets := map[string]appsv1.DeploymentSpec{}
	for _, name := range rotationComponents {
		desired := prepared.deployments[name]
		count, found := replicas[name]
		if desired == nil || !found || count < 1 || desired.Spec.Replicas == nil || *desired.Spec.Replicas != count {
			return nil, errors.New("provider target must preserve every pinned replica count")
		}
		current, err := s.kube.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if !rotationOwned(current, namespace, radiusReleaseName) || current.Spec.Paused ||
			!apiequality.Semantic.DeepEqual(current.Spec.Selector, desired.Spec.Selector) {
			return nil, errors.New("provider target cannot change workload ownership, pause state or selectors")
		}
		candidate := current.DeepCopy()
		candidate.Spec = *desired.Spec.DeepCopy()
		defaulted, err := s.kube.AppsV1().Deployments(namespace).Update(ctx, candidate,
			metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}, FieldManager: FieldManager})
		if err != nil {
			return nil, fmt.Errorf("cannot validate target Deployment %s without writing it: %w", name, err)
		}
		if defaulted.UID != current.UID || defaulted.Spec.Replicas == nil || *defaulted.Spec.Replicas != count ||
			defaulted.Spec.Paused || !apiequality.Semantic.DeepEqual(defaulted.Spec.Selector, current.Spec.Selector) {
			return nil, errors.New("admission changed provider workload identity, replicas, pause state or selectors")
		}
		targets[name] = *defaulted.Spec.DeepCopy()
	}
	return targets, nil
}
