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

package container

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	k8s "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/radius-project/radius/pkg/corerp/handlers"
	"github.com/radius-project/radius/pkg/kubernetes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
)

const (
	// SecretHashMigrationStepName is the name of the migration step that records secret hashes on Deployments.
	SecretHashMigrationStepName = "containerSecretHash"

	// secretHashFieldManager owns the recorded secret hash. Radius applies containers with another field manager
	// that never sets the annotation, so applying a container never conflicts with it. Only the migration writes the
	// annotation, so it takes ownership of the annotation if another writer set it.
	secretHashFieldManager = "radius-sha256-migration"
)

// NewSecretHashTransformer returns a transformer for the Deployments of containers. Older versions of Radius
// computed the secret hash in the Pod template with SHA-1. While the secret data is unchanged, the transformer
// keeps that hash instead of the current one, so upgrading Radius does not roll out workloads. When the secret
// data changes, the current hash replaces it and the workload rolls out as it always has.
//
// reader reads the cluster the Deployment is applied to. See https://github.com/radius-project/radius/issues/8084.
func NewSecretHashTransformer(reader client.Reader) func(context.Context, *handlers.PutOptions) error {
	return func(ctx context.Context, options *handlers.PutOptions) error {
		deployment, ok := options.Resource.CreateResource.Data.(*appsv1.Deployment)
		if !ok || reader == nil {
			return nil
		}

		desired := deployment.Spec.Template.Annotations[kubernetes.AnnotationSecretHash]
		if desired == "" {
			return nil
		}

		live := &appsv1.Deployment{}
		err := reader.Get(ctx, client.ObjectKey{Namespace: deployment.Namespace, Name: deployment.Name}, live)
		if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
			// A new Deployment, or one Radius cannot read, gets the current hash.
			return nil
		} else if err != nil {
			return fmt.Errorf("failed to read Deployment %s/%s: %w", deployment.Namespace, deployment.Name, err)
		}

		legacy := live.Spec.Template.Annotations[kubernetes.AnnotationSecretHash]
		if !kubernetes.IsLegacySecretHash(legacy) {
			return nil
		}

		unchanged, err := legacySecretHashMatches(ctx, reader, options, deployment, live, legacy, desired)
		if err != nil {
			return err
		}

		if unchanged {
			deployment.Spec.Template.Annotations[kubernetes.AnnotationSecretHash] = legacy
		}

		return nil
	}
}

// legacySecretHashMatches reports whether the legacy hash in the live Pod template was computed from the same
// secret data as the desired hash.
func legacySecretHashMatches(ctx context.Context, reader client.Reader, options *handlers.PutOptions, deployment *appsv1.Deployment, live *appsv1.Deployment, legacy string, desired string) (bool, error) {
	// The SHA-256 migration records the current hash of the data the legacy hash was computed from. A record of
	// another legacy hash describes other data, for example after a rollback deployed a different secret, so it is
	// ignored and the secret is checked instead.
	if recordedLegacy, recorded, ok := kubernetes.ParseRecordedSecretHash(live.Annotations[kubernetes.AnnotationSecretHashSHA256]); ok && recordedLegacy == legacy {
		return recorded == desired, nil
	}

	key := client.ObjectKey{Namespace: deployment.Namespace, Name: deployment.Name}
	if properties := options.DependencyProperties[rpv1.LocalIDSecret]; properties[handlers.ResourceName] != "" {
		key = client.ObjectKey{Namespace: properties[handlers.KubernetesNamespaceKey], Name: properties[handlers.ResourceName]}
	}

	// The Deployment depends on its secret, which is applied first, so the live secret holds the desired data.
	secret := &corev1.Secret{}
	err := reader.Get(ctx, key, secret)
	if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
		// Without the secret the legacy hash cannot be verified, so the current hash replaces it.
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("failed to read Secret %s/%s: %w", key.Namespace, key.Name, err)
	}

	return kubernetes.HashSecretData(secret.Data) == desired && kubernetes.LegacyHashSecretData(secret.Data) == legacy, nil
}

// NewSecretHashMigrationStep returns the migration step that prepares container workloads for the release that
// removes SHA-1. client returns a client for the cluster the Deployments of containers are applied to.
func NewSecretHashMigrationStep(client func(context.Context) (k8s.Interface, error)) hashmigration.Step {
	return hashmigration.StepFunc{
		StepName: SecretHashMigrationStepName,
		Fn: func(ctx context.Context) (hashmigration.Result, error) {
			kubernetesClient, err := client(ctx)
			if err != nil {
				return hashmigration.Result{}, err
			}

			return RecordSecretHashes(ctx, kubernetesClient)
		},
	}
}

// RecordSecretHashes records the current secret hash on every Deployment of a container whose Pod template still
// carries the legacy secret hash of its unchanged secret. The hash is recorded with the legacy hash in the
// AnnotationSecretHashSHA256 annotation of the Deployment, outside the Pod template, so recording it does not roll out
// the workload: Kubernetes advances the Deployment's generation for the annotation, but keeps its ReplicaSet and pods.
// With it, NewSecretHashTransformer keeps the legacy hash without computing SHA-1. A record of another legacy hash,
// left when a rollback deployed different secret data, is replaced.
func RecordSecretHashes(ctx context.Context, client k8s.Interface) (hashmigration.Result, error) {
	result := hashmigration.Result{}
	options := metav1.ListOptions{LabelSelector: kubernetes.LabelRadiusResource, Limit: 250}
	for {
		list, err := client.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, options)
		if err != nil {
			return result, fmt.Errorf("failed to list Deployments: %w", err)
		}

		for index := range list.Items {
			recorded := recordSecretHash(ctx, client, &list.Items[index])
			result.Add(recorded)
			if recorded.Message != "" {
				result.Message = recorded.Message
			}
		}

		if list.Continue == "" {
			return result, nil
		}

		options.Continue = list.Continue
	}
}

func recordSecretHash(ctx context.Context, client k8s.Interface, deployment *appsv1.Deployment) hashmigration.Result {
	legacy := deployment.Spec.Template.Annotations[kubernetes.AnnotationSecretHash]
	if !kubernetes.IsLegacySecretHash(legacy) {
		return hashmigration.Result{}
	}

	if recordedLegacy, _, ok := kubernetes.ParseRecordedSecretHash(deployment.Annotations[kubernetes.AnnotationSecretHashSHA256]); ok && recordedLegacy == legacy {
		return hashmigration.Result{}
	}

	logger := ucplog.FromContextOrDiscard(ctx).WithValues("deployment", deployment.Name, "namespace", deployment.Namespace)

	// The container renderer names the secret after the Deployment.
	secret, err := client.CoreV1().Secrets(deployment.Namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return hashmigration.Result{Skipped: 1, Message: fmt.Sprintf("the secret of Deployment %s/%s does not exist", deployment.Namespace, deployment.Name)}
	} else if err != nil {
		logger.Error(err, "Unable to read the secret of the Deployment; recording its secret hash will be retried")
		return hashmigration.Result{Remaining: 1, Message: err.Error()}
	}

	if kubernetes.LegacyHashSecretData(secret.Data) != legacy {
		// The secret changed after the workload was deployed. The next deployment rolls out the workload, as it
		// would have before.
		return hashmigration.Result{Skipped: 1, Message: fmt.Sprintf("the secret of Deployment %s/%s changed after it was deployed", deployment.Namespace, deployment.Name)}
	}

	apply := appsv1ac.Deployment(deployment.Name, deployment.Namespace).
		WithResourceVersion(deployment.ResourceVersion).
		WithAnnotations(map[string]string{kubernetes.AnnotationSecretHashSHA256: kubernetes.RecordSecretHash(legacy, kubernetes.HashSecretData(secret.Data))})
	_, err = client.AppsV1().Deployments(deployment.Namespace).Apply(ctx, apply, metav1.ApplyOptions{FieldManager: secretHashFieldManager, Force: true})
	if err != nil {
		// A conflict means the Deployment changed since it was listed. Either way, the next pass looks again.
		logger.Info("Unable to record the secret hash of the Deployment; it will be retried", "reason", err.Error())
		return hashmigration.Result{Remaining: 1, Message: err.Error()}
	}

	logger.Info("Recorded the secret hash of the Deployment")
	return hashmigration.Result{Migrated: 1}
}
