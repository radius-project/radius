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
	"fmt"

	helm "helm.sh/helm/v4/pkg/action"
	releaseutil "helm.sh/helm/v4/pkg/release/v1/util"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

// strategyFieldManager is the field manager for the change that clears a live Deployment's rollingUpdate. rad
// removes its managedFields entry afterwards, so the server-side apply that follows solely owns the strategy.
const strategyFieldManager = "rad-strategy"

// recreateStrategyPreparer is a Helm post-renderer that prepares live Deployments for a server-side apply that
// changes their strategy to Recreate. It returns the manifests unchanged.
//
// A Deployment created without a strategy is defaulted to RollingUpdate with a rollingUpdate field that no field
// manager owns. Server-side apply only removes fields its manager owns, so applying type Recreate keeps the
// defaulted rollingUpdate, and the API server rejects the result. Client-side apply doesn't need this: its
// strategic merge patch replaces the strategy.
type recreateStrategyPreparer struct {
	helmConf  *helm.Configuration
	namespace string
}

// withRecreateStrategyPreparer configures upgradeClient to prepare Recreate Deployments before it applies them with
// server-side apply. Helm runs post-renderers after rendering and before applying, for upgrades and rollbacks.
func withRecreateStrategyPreparer(upgradeClient *helm.Upgrade, helmConf *helm.Configuration, serverSideApply bool) *helm.Upgrade {
	if !serverSideApply {
		return upgradeClient
	}
	upgradeClient.PostRenderer = &recreateStrategyPreparer{helmConf: helmConf, namespace: upgradeClient.Namespace}
	upgradeClient.PostRenderStrategy = helm.PostRenderStrategyNoHooks
	return upgradeClient
}

// Run clears the rollingUpdate of each live Deployment that the manifests change to Recreate.
func (p *recreateStrategyPreparer) Run(manifests *bytes.Buffer) (*bytes.Buffer, error) {
	deployments, err := recreateDeployments(manifests.String(), p.namespace)
	if err != nil {
		return nil, err
	}
	// Configurations without a REST client getter, such as fakes in tests, don't apply to a cluster.
	if len(deployments) == 0 || p.helmConf == nil || p.helmConf.RESTClientGetter == nil {
		return manifests, nil
	}

	client, err := p.helmConf.KubernetesClientSet()
	if err != nil {
		return nil, err
	}
	for _, deployment := range deployments {
		if err := clearRollingUpdate(context.Background(), client, deployment.Namespace, deployment.Name); err != nil {
			return nil, fmt.Errorf("failed to prepare Deployment %s/%s for the Recreate strategy: %w", deployment.Namespace, deployment.Name, err)
		}
	}
	return manifests, nil
}

// recreateDeployments returns the namespace and name of each apps/v1 Deployment in manifests with the Recreate
// strategy. Deployments without a namespace are in defaultNamespace.
func recreateDeployments(manifests, defaultNamespace string) ([]types.NamespacedName, error) {
	var deployments []types.NamespacedName
	for _, manifest := range releaseutil.SplitManifests(manifests) {
		var typeMeta metav1.TypeMeta
		if err := yaml.Unmarshal([]byte(manifest), &typeMeta); err != nil {
			return nil, fmt.Errorf("failed to parse rendered manifest: %w", err)
		}
		if typeMeta.APIVersion != appsv1.SchemeGroupVersion.String() || typeMeta.Kind != "Deployment" {
			continue
		}
		var deployment appsv1.Deployment
		if err := yaml.Unmarshal([]byte(manifest), &deployment); err != nil {
			return nil, fmt.Errorf("failed to parse rendered Deployment: %w", err)
		}
		if deployment.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType {
			continue
		}
		namespace := deployment.Namespace
		if namespace == "" {
			namespace = defaultNamespace
		}
		deployments = append(deployments, types.NamespacedName{Namespace: namespace, Name: deployment.Name})
	}
	return deployments, nil
}

// clearRollingUpdate changes a live Deployment that has a rollingUpdate to Recreate without one, then removes the
// change's managedFields entry. Without that entry, the server-side apply that follows solely owns the strategy, so
// a later apply without a strategy restores the default, as it does for releases that use client-side apply.
// Changing only the strategy doesn't roll out new pods.
func clearRollingUpdate(ctx context.Context, client kubernetes.Interface, namespace, name string) error {
	deployments := client.AppsV1().Deployments(namespace)
	live, err := deployments.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	if live.Spec.Strategy.RollingUpdate == nil {
		return nil
	}

	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{"resourceVersion": live.ResourceVersion},
		"spec": map[string]any{"strategy": map[string]any{
			"type":          appsv1.RecreateDeploymentStrategyType,
			"rollingUpdate": nil,
		}},
	})
	if err != nil {
		return err
	}
	updated, err := deployments.Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{FieldManager: strategyFieldManager})
	if err != nil {
		return err
	}

	for i, entry := range updated.ManagedFields {
		if entry.Manager != strategyFieldManager || entry.Operation != metav1.ManagedFieldsOperationUpdate {
			continue
		}
		path := fmt.Sprintf("/metadata/managedFields/%d", i)
		patch, err := json.Marshal([]map[string]any{
			{"op": "test", "path": path + "/manager", "value": strategyFieldManager},
			{"op": "remove", "path": path},
		})
		if err != nil {
			return err
		}
		_, err = deployments.Patch(ctx, name, types.JSONPatchType, patch, metav1.PatchOptions{FieldManager: strategyFieldManager})
		return err
	}
	return nil
}
