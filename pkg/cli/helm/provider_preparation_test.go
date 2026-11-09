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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kubetesting "k8s.io/client-go/testing"
)

func TestProviderPreparationChart(t *testing.T) {
	t.Parallel()
	for _, provider := range []string{"radius", "external"} {
		for _, mode := range []string{"off", "dryRun", "enforce"} {
			t.Run(provider+"/"+mode, func(t *testing.T) {
				t.Parallel()
				selected, err := loader.Load("../../../deploy/Chart")
				require.NoError(t, err)
				generation := strings.Repeat("a", 32)
				values := map[string]any{
					"dynamicrp": map[string]any{"buildkit": map[string]any{"enabled": true}},
					"global": map[string]any{"rbac": map[string]any{
						"providerGeneration": generation, "enabled": mode == "enforce", "dryRun": mode == "dryRun",
						"mtls": map[string]any{"provider": provider},
					}},
					"de": map[string]any{"image": "test.invalid/paired-engine", "tag": "local",
						"externalIdentity": map[string]any{"enabled": provider == "external"},
					},
				}
				impl := &Impl{contourCluster: &fakeContourCluster{}}
				options := PopulateDefaultClusterOptions(CLIClusterOptions{})
				before, err := providerHash(selected)
				require.NoError(t, err)
				prepared, err := impl.renderProviderPreparation(t.Context(), selected, options, values, "")
				require.NoError(t, err)
				require.Equal(t, 5, len(prepared.deployments))
				require.Equal(t, 6, len(prepared.configurations))
				for _, component := range rotationComponents {
					deployment := prepared.deployments[component]
					require.NotNil(t, deployment)
					require.Contains(t, providerConfigurationNames(deployment.Spec.Template.Spec), component+"-config-"+generation)
					require.Contains(t, prepared.configurations, component+"-config-"+generation)
				}
				discoveryCount := 0
				if provider == "external" && mode != "off" {
					discoveryCount = 2
				}
				require.Len(t, prepared.discovery, discoveryCount)
				for _, object := range prepared.discovery {
					broader := object.DeepCopy()
					if broader.GetKind() == "ClusterRole" {
						broader.Object["rules"] = []any{map[string]any{"resources": []any{"*"}, "verbs": []any{"*"}, "apiGroups": []any{"*"}}}
					} else {
						require.NoError(t, unstructured.SetNestedField(broader.Object, "cluster-admin", "roleRef", "name"))
					}
					require.Error(t, validateProviderDiscovery(broader, options.Radius.Namespace))
				}
				after, err := providerHash(selected)
				require.NoError(t, err)
				require.Equal(t, before, after, "preparation must not weaken or mutate the source chart")
				for _, bad := range []any{"", "INVALID", false, strings.Repeat("A", 32)} {
					broken := copyValues(values)
					require.NoError(t, unstructured.SetNestedField(broken, bad, "global", "rbac", providerGenerationValue))
					_, err := impl.renderProviderPreparation(t.Context(), selected, options, broken, "")
					require.Error(t, err)
				}
			})
		}
	}
}

func TestProviderConfigurationPreparationRetry(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"success", "interrupted", "foreign", "drift", "live missing", "guard denied"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			store, plan, _, kube := providerFakeFixture(t, "radius", "external", "enforce")
			generation := strings.Repeat("a", 32)
			prepared := providerPreparedResources{configurations: map[string]*corev1.ConfigMap{}}
			for _, base := range providerConfigurationBases[:5] {
				name := base + "-" + generation
				prepared.configurations[name] = &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: plan.Namespace}, Data: map[string]string{"config": "target"},
				}
			}
			name := "controller-config-" + generation
			if scenario == "foreign" || scenario == "drift" {
				existing := prepared.configurations[name].DeepCopy()
				if scenario == "drift" {
					existing.Annotations = map[string]string{"meta.helm.sh/release-name": radiusReleaseName,
						"meta.helm.sh/release-namespace": plan.Namespace}
					existing.Labels = map[string]string{"app.kubernetes.io/managed-by": "Helm"}
					existing.Data["config"] = "changed"
				}
				_, err := kube.CoreV1().ConfigMaps(plan.Namespace).Create(t.Context(), existing, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			if scenario == "live missing" {
				deployment, err := kube.AppsV1().Deployments(plan.Namespace).Get(t.Context(), "controller", metav1.GetOptions{})
				require.NoError(t, err)
				deployment.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: "config", VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}},
				}}}
				_, err = kube.AppsV1().Deployments(plan.Namespace).Update(t.Context(), deployment, metav1.UpdateOptions{})
				require.NoError(t, err)
			}
			attempts := 0
			interrupted := false
			kube.PrependReactor("create", "configmaps", func(action kubetesting.Action) (bool, runtime.Object, error) {
				attempts++
				if scenario == "interrupted" && attempts == 2 && !interrupted {
					interrupted = true
					return true, nil, errors.New("injected partial preparation")
				}
				return false, nil, nil
			})
			kube.ClearActions()
			guard := func(context.Context) error {
				if scenario == "guard denied" {
					return errors.New("lost lease")
				}
				return nil
			}
			err := store.prepareConfigurations(t.Context(), plan.Namespace, prepared, guard)
			if scenario == "interrupted" {
				require.Error(t, err)
				err = store.prepareConfigurations(t.Context(), plan.Namespace, prepared, guard)
			}
			if scenario == "success" || scenario == "interrupted" {
				require.NoError(t, err)
				count := attempts
				require.NoError(t, store.prepareConfigurations(t.Context(), plan.Namespace, prepared, guard))
				require.Equal(t, count, attempts, "retry reuses exact owned preparations without another write")
			} else {
				require.Error(t, err)
				require.Zero(t, attempts, "known conflicts must fail before preparation writes")
			}
			for _, action := range kube.Actions() {
				require.NotEqual(t, "update", action.GetVerb())
				require.NotEqual(t, "patch", action.GetVerb())
				require.NotEqual(t, "delete", action.GetVerb())
			}
		})
	}
}
