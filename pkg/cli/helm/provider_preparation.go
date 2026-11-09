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
	"slices"
	"strings"

	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

const providerGenerationValue = "providerGeneration"

var providerConfigurationBases = []string{
	"ucp-config", "applications-rp-config", "dynamic-rp-config", "controller-config", "bicep-de-config",
	"dynamic-rp-buildkit-config",
}

type providerPreparedResources struct {
	deployments    map[string]*appsv1.Deployment
	configurations map[string]*corev1.ConfigMap
	discovery      []*unstructured.Unstructured
}

// Render only preparation resources, never PKI or the APIService. The caller
// separately validates the selected CA, issuer policies and source lifecycle.
// Full Helm rendering retains its lifecycle gates until the transaction finishes.
func (i *Impl) renderProviderPreparation(ctx context.Context, selected *chart.Chart, options ClusterOptions,
	values map[string]any, kubeContext string) (providerPreparedResources, error) {
	copy := cloneChart(selected)
	copy.Templates = nil
	for _, file := range selected.Templates {
		if strings.HasPrefix(file.Name, "templates/_") ||
			slices.Contains([]string{"templates/ucp/deployment.yaml", "templates/ucp/configmaps.yaml",
				"templates/rp/deployment.yaml", "templates/rp/configmaps.yaml",
				"templates/dynamic-rp/deployment.yaml", "templates/dynamic-rp/configmaps.yaml",
				"templates/dynamic-rp/buildkit-config.yaml", "templates/controller/deployment.yaml",
				"templates/controller/configmaps.yaml", "templates/de/deployment.yaml", "templates/de/configmaps.yaml",
				"templates/global/external-identity.yaml"}, file.Name) {
			copy.Templates = append(copy.Templates, &common.File{Name: file.Name, Data: slices.Clone(file.Data)})
		}
	}
	manifest, err := i.renderBootstrapChart(ctx, copy, options, values, kubeContext)
	if err != nil {
		return providerPreparedResources{}, err
	}
	generation, _, err := unstructured.NestedString(values, "global", "rbac", providerGenerationValue)
	if err != nil {
		return providerPreparedResources{}, err
	}
	return decodeProviderPreparation(manifest, options.Radius.Namespace, generation)
}

func decodeProviderPreparation(manifest, namespace, generation string) (providerPreparedResources, error) {
	result := providerPreparedResources{deployments: map[string]*appsv1.Deployment{}, configurations: map[string]*corev1.ConfigMap{}}
	if len(generation) != 32 || !providerDigestValid(generation+generation) {
		return result, errors.New("provider preparation requires an exact separate configuration generation")
	}
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	seen := map[string]bool{}
	for {
		object := &unstructured.Unstructured{}
		if err := decoder.Decode(object); err == io.EOF {
			break
		} else if err != nil {
			return result, errors.New("invalid provider preparation manifest")
		}
		if len(object.Object) == 0 {
			continue
		}
		key := object.GetKind() + "/" + object.GetName()
		if seen[key] || object.GetDeletionTimestamp() != nil || len(object.GetOwnerReferences()) != 0 {
			return result, errors.New("provider preparation repeats a resource or contains an invalid owner")
		}
		seen[key] = true
		switch object.GetKind() {
		case "Deployment":
			if object.GetAPIVersion() != "apps/v1" || object.GetNamespace() != namespace ||
				!slices.Contains(rotationComponents, object.GetName()) {
				return result, errors.New("provider preparation contains an unexpected workload")
			}
			deployment := &appsv1.Deployment{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, deployment); err != nil {
				return result, errors.New("invalid provider workload schema")
			}
			if deployment.Spec.Template.Spec.ServiceAccountName != deployment.Name {
				return result, errors.New("provider workload uses a different ServiceAccount")
			}
			result.deployments[deployment.Name] = deployment
		case "ConfigMap":
			base, found := strings.CutSuffix(object.GetName(), "-"+generation)
			if !found || !slices.Contains(providerConfigurationBases, base) ||
				object.GetNamespace() != namespace || object.GetAPIVersion() != "v1" {
				return result, errors.New("provider configuration is not in its separate generation")
			}
			config := &corev1.ConfigMap{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, config); err != nil {
				return result, errors.New("invalid provider configuration schema")
			}
			result.configurations[config.Name] = config
		case "ClusterRole", "ClusterRoleBinding":
			if object.GetAPIVersion() != "rbac.authorization.k8s.io/v1" || object.GetNamespace() != "" ||
				object.GetName() != "radius-token-discovery-"+namespace {
				return result, errors.New("provider preparation contains unexpected cluster permissions")
			}
			if err := validateProviderDiscovery(object, namespace); err != nil {
				return result, err
			}
			result.discovery = append(result.discovery, object)
		default:
			return result, errors.New("provider preparation must not contain CA, Secret, routing or unrelated resources")
		}
	}
	if len(result.deployments) != len(rotationComponents) {
		return result, errors.New("provider preparation requires all five workloads")
	}
	for _, deployment := range result.deployments {
		for _, name := range providerConfigurationNames(deployment.Spec.Template.Spec) {
			if slices.Contains(providerConfigurationBases, name) {
				return result, errors.New("provider preparation still references live source configuration")
			}
			if strings.HasSuffix(name, "-"+generation) && result.configurations[name] == nil {
				return result, errors.New("provider preparation is missing a generated configuration")
			}
		}
	}
	for _, base := range providerConfigurationBases[:5] {
		if result.configurations[base+"-"+generation] == nil {
			return result, errors.New("provider preparation is missing a component configuration")
		}
	}
	return result, nil
}

func validateProviderDiscovery(object *unstructured.Unstructured, namespace string) error {
	fail := errors.New("provider preparation may grant only fixed public signing-key discovery to canonical components")
	if object.GetKind() == "ClusterRole" {
		role := &rbacv1.ClusterRole{}
		if runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, role) != nil ||
			role.AggregationRule != nil || !apiequality.Semantic.DeepEqual(role.Rules, []rbacv1.PolicyRule{{
			NonResourceURLs: []string{"/.well-known/openid-configuration", "/openid/v1/jwks"}, Verbs: []string{"get"},
		}}) {
			return fail
		}
		return nil
	}
	binding := &rbacv1.ClusterRoleBinding{}
	if runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, binding) != nil ||
		binding.RoleRef != (rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "radius-token-discovery-" + namespace}) ||
		len(binding.Subjects) != len(rotationComponents) {
		return fail
	}
	seen := map[string]bool{}
	for _, subject := range binding.Subjects {
		if subject.Kind != "ServiceAccount" || subject.APIGroup != "" || subject.Namespace != namespace ||
			!slices.Contains(rotationComponents, subject.Name) || seen[subject.Name] {
			return fail
		}
		seen[subject.Name] = true
	}
	return nil
}

// Preparation is create-only. Retry may reuse only the exact owned content;
// it cannot overwrite either live source configuration or foreign resources.
func (s *providerTransitionStore) prepareConfigurations(ctx context.Context, namespace string,
	prepared providerPreparedResources, beforeWrite func(context.Context) error) error {
	if beforeWrite == nil {
		return errors.New("provider preparation requires the shared operation write guard")
	}
	referenced := map[string]bool{}
	for _, component := range rotationComponents {
		deployment, err := s.kube.AppsV1().Deployments(namespace).Get(ctx, component, metav1.GetOptions{})
		if err != nil {
			return err
		}
		for _, name := range providerConfigurationNames(deployment.Spec.Template.Spec) {
			referenced[name] = true
		}
	}
	client := s.kube.CoreV1().ConfigMaps(namespace)
	var missing []*corev1.ConfigMap
	for _, name := range slices.Sorted(maps.Keys(prepared.configurations)) {
		desired := prepared.configurations[name].DeepCopy()
		if desired.Name != name || desired.Namespace != namespace || slices.Contains(providerConfigurationBases, name) {
			return errors.New("provider preparation cannot write live source configuration")
		}
		desired.UID, desired.ResourceVersion = "", ""
		if desired.Labels == nil {
			desired.Labels = map[string]string{}
		}
		if desired.Annotations == nil {
			desired.Annotations = map[string]string{}
		}
		desired.Labels["app.kubernetes.io/managed-by"] = "Helm"
		desired.Annotations["meta.helm.sh/release-name"] = radiusReleaseName
		desired.Annotations["meta.helm.sh/release-namespace"] = namespace
		current, err := client.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			if referenced[name] {
				return errors.New("provider preparation cannot recreate configuration referenced by a live workload")
			}
			missing = append(missing, desired)
			continue
		}
		if err != nil {
			return err
		}
		if err := validatePreparedConfiguration(current, desired, namespace); err != nil {
			return err
		}
	}
	for _, desired := range missing {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := beforeWrite(ctx); err != nil {
			return err
		}
		current, err := client.Create(ctx, desired, metav1.CreateOptions{FieldManager: FieldManager})
		if apierrors.IsAlreadyExists(err) {
			current, err = client.Get(ctx, desired.Name, metav1.GetOptions{})
		}
		if err != nil {
			return err
		}
		if err := validatePreparedConfiguration(current, desired, namespace); err != nil {
			return err
		}
	}
	return nil
}

func validatePreparedConfiguration(current, desired *corev1.ConfigMap, namespace string) error {
	if current.Name != desired.Name || current.Namespace != namespace || current.UID == "" || current.ResourceVersion == "" ||
		current.DeletionTimestamp != nil || len(current.OwnerReferences) != 0 ||
		!rotationOwned(current, namespace, radiusReleaseName) || !maps.Equal(current.Data, desired.Data) ||
		!rotationJSONEqual(current.BinaryData, desired.BinaryData) || !apiequality.Semantic.DeepEqual(current.Immutable, desired.Immutable) {
		return fmt.Errorf("prepared configuration %s has different ownership or content; it will not be overwritten", desired.Name)
	}
	return nil
}
