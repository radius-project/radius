/*
Copyright 2023 The Radius Authors.

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

package kubeutil

import (
	"context"
	"fmt"
	"maps"
	"os"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/kubernetes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/retry"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

// PatchNamespace creates a new namespace with the given name and labels it with the given label, then applies it to the
// cluster. It returns an error if the patch operation fails.
func PatchNamespace(ctx context.Context, client runtime_client.Client, namespace string) error {
	ns := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name": namespace,
				"labels": map[string]any{
					kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
				},
			},
		},
	}

	// Using runtime_client.Apply patch type for server-side apply with namespace resources.
	// The new client.Client.Apply() API requires ApplyConfiguration types which are
	// not straightforward to generate for dynamic/unstructured resources.
	//nolint:staticcheck // SA1019: runtime_client.Apply will be replaced when ApplyConfiguration support is available
	err := client.Patch(ctx, ns, runtime_client.Apply, &runtime_client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return fmt.Errorf("error applying namespace: %w", err)
	}

	return nil
}

// CreateNamespace creates a namespace for an environment or application with the Pod Security labels
// selected by opts. When the namespace already exists, it adds only the Pod Security labels that are
// not already set, so labels an operator set are never changed. In off mode it creates the namespace
// without labels and does not read or change an existing namespace. It returns whether the namespace
// was created and the labels Radius added.
func CreateNamespace(ctx context.Context, client runtime_client.Client, name string, opts authz.Options) (bool, map[string]string, error) {
	labels := opts.PodSecurityLabels(name)
	err := client.Create(ctx, &corev1.Namespace{Name: name, Labels: labels})
	if err == nil {
		return true, labels, nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return false, nil, err
	}
	if len(labels) == 0 {
		return false, nil, nil
	}

	ns := &corev1.Namespace{Name: name}
	added, err := AddMissingNamespaceLabels(ctx, client, ns, labels)
	return false, added, err
}

// AddMissingNamespaceLabels adds the entries of labels whose keys are not already set on ns and
// returns the labels it added. Conflicting patches retry against fresh labels so concurrent operator
// changes are preserved.
func AddMissingNamespaceLabels(ctx context.Context, client runtime_client.Client, ns *corev1.Namespace, labels map[string]string) (map[string]string, error) {
	if len(labels) == 0 {
		return nil, nil
	}

	var added map[string]string
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := client.Get(ctx, runtime_client.ObjectKey{Name: ns.Name}, ns); err != nil {
			return err
		}
		missing := authz.MissingLabels(ns.Labels, labels)
		if len(missing) == 0 {
			return nil
		}

		base := ns.DeepCopy()
		if ns.Labels == nil {
			ns.Labels = map[string]string{}
		}
		maps.Copy(ns.Labels, missing)

		patch := runtime_client.MergeFromWithOptions(base, runtime_client.MergeFromWithOptimisticLock{})
		if err := client.Patch(ctx, ns, patch); err != nil {
			return err
		}
		added = missing
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add Pod Security labels to namespace %q: %w", ns.Name, err)
	}

	return added, nil
}

// DeploymentTargetAuthorization scopes namespace admission and Pod Security labels to the
// deployment target. A kubeconfig can point back at the control-plane cluster through a
// different address, so only different live kube-system namespace UIDs establish separation.
// Identity is needed only for a namespace that would otherwise be protected.
func DeploymentTargetAuthorization(ctx context.Context, opts authz.Options, controlPlane, target runtime_client.Client, namespace string) (authz.Options, error) {
	if opts.EffectiveMode() == authz.ModeOff || !opts.IsControlPlaneNamespace(namespace) || os.Getenv(TargetKubeconfigEnvVar) == "" {
		return opts, nil
	}
	if controlPlane == nil || target == nil {
		return opts, fmt.Errorf("cannot identify deployment target without both Kubernetes clients")
	}

	controlPlaneSystem := &corev1.Namespace{}
	if err := controlPlane.Get(ctx, runtime_client.ObjectKey{Name: "kube-system"}, controlPlaneSystem); err != nil {
		return opts, fmt.Errorf("failed to identify control-plane cluster: %w", err)
	}
	targetSystem := &corev1.Namespace{}
	if err := target.Get(ctx, runtime_client.ObjectKey{Name: "kube-system"}, targetSystem); err != nil {
		return opts, fmt.Errorf("failed to identify deployment-target cluster: %w", err)
	}
	if controlPlaneSystem.UID == "" || targetSystem.UID == "" {
		return opts, fmt.Errorf("cannot identify deployment target: kube-system namespace UID is empty")
	}

	opts.TargetClusterIsDistinct = controlPlaneSystem.UID != targetSystem.UID
	return opts, nil
}
