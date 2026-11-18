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
	"io"
	"strings"

	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	kubefake "helm.sh/helm/v4/pkg/kube/fake"
	"helm.sh/helm/v4/pkg/storage/driver"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	typedapps "k8s.io/client-go/kubernetes/typed/apps/v1"
)

// MigrateControllerStrategy removes defaulted RollingUpdate fields that server-side apply cannot clear with null.
// Only the Radius entry points call this; generic Helm operations (including Contour and rollback) are unchanged.
func (client *HelmClientImpl) MigrateControllerStrategy(ctx context.Context, configuration *helm.Configuration, helmChart *chart.Chart, values map[string]any, releaseName, namespace string, reuseValues bool) error {
	history, err := client.RunHelmHistory(configuration, releaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read Radius release history: %w", err)
	}

	// Match Helm's baseline selection, value merging, capabilities and lookups without writing release history
	// or applying resources. In particular, a failed last revision need not be the values baseline.
	renderConfig := &helm.Configuration{
		RESTClientGetter:    configuration.RESTClientGetter,
		RegistryClient:      configuration.RegistryClient,
		Capabilities:        configuration.Capabilities,
		CustomTemplateFuncs: configuration.CustomTemplateFuncs,
		KubeClient:          &kubefake.PrintingKubeClient{Out: io.Discard},
	}
	renderConfig.Releases, err = releaseHistoryStorage(history)
	if err != nil {
		return fmt.Errorf("failed to copy Radius release history: %w", err)
	}
	upgrade := newUpgradeClient(renderConfig, namespace, false, reuseValues, 0, usesServerSideApply(configuration, releaseName))
	upgrade.DryRunStrategy = helm.DryRunServer
	rendered, err := upgrade.RunWithContext(ctx, releaseName, cloneChart(helmChart), copyValues(values))
	if err != nil {
		return fmt.Errorf("failed to render Radius controller upgrade: %w", err)
	}
	revision, err := asRelease(rendered)
	if err != nil {
		return err
	}
	recreate, err := controllerUsesRecreate(revision.Manifest, namespace)
	if err != nil {
		return fmt.Errorf("failed to read rendered Radius controller: %w", err)
	}
	if !recreate {
		return nil
	}

	config, err := configuration.RESTClientGetter.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("failed to get Radius Kubernetes configuration: %w", err)
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Radius Kubernetes client: %w", err)
	}
	return migrateControllerDeployment(ctx, kubeClient.AppsV1().Deployments(namespace), releaseName, namespace)
}

func controllerUsesRecreate(manifest, namespace string) (bool, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		var object unstructured.Unstructured
		if err := decoder.Decode(&object); err != nil {
			if err == io.EOF {
				return false, nil
			}
			return false, err
		}
		if object.GetAPIVersion() != "apps/v1" || object.GetKind() != "Deployment" || object.GetName() != "controller" {
			continue
		}
		if object.GetNamespace() != "" && object.GetNamespace() != namespace {
			continue
		}
		strategy, _, err := unstructured.NestedString(object.Object, "spec", "strategy", "type")
		return strategy == string(appsv1.RecreateDeploymentStrategyType), err
	}
}

func migrateControllerDeployment(ctx context.Context, deployments typedapps.DeploymentInterface, releaseName, namespace string) error {
	deployment, err := deployments.Get(ctx, "controller", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read controller Deployment in namespace %q: %w", namespace, err)
	}
	if deployment.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType && deployment.Spec.Strategy.RollingUpdate == nil {
		return nil
	}
	if deployment.Labels["app.kubernetes.io/managed-by"] != "Helm" ||
		deployment.Annotations["meta.helm.sh/release-name"] != releaseName ||
		deployment.Annotations["meta.helm.sh/release-namespace"] != namespace {
		return fmt.Errorf("controller Deployment in namespace %q is not owned by Helm release %q; refusing to change its strategy", namespace, releaseName)
	}

	// Changing strategy alone leaves the running pod and replica count untouched. UID and resourceVersion tests
	// prevent overwriting a concurrently edited or replaced Deployment; the user can retry after a conflict.
	patch, err := json.Marshal([]map[string]any{
		{"op": "test", "path": "/metadata/uid", "value": deployment.UID},
		{"op": "test", "path": "/metadata/resourceVersion", "value": deployment.ResourceVersion},
		{"op": "add", "path": "/spec/strategy", "value": appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}},
	})
	if err != nil {
		return fmt.Errorf("failed to encode controller strategy migration: %w", err)
	}
	if _, err := deployments.Patch(ctx, deployment.Name, types.JSONPatchType, patch, metav1.PatchOptions{FieldManager: FieldManager}); err != nil {
		return fmt.Errorf("failed to migrate controller Deployment in namespace %q to Recreate; retry after resolving the error: %w", namespace, err)
	}
	return nil
}
