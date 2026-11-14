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
	"time"

	chart "helm.sh/helm/v4/pkg/chart/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

func (i *Impl) rotationStoreForContext(kubeContext, namespace string) (*rotationStore, error) {
	if i.newRotationStore != nil {
		return i.newRotationStore(kubeContext, namespace)
	}
	flags := genericclioptions.NewConfigFlags(false)
	flags.Context, flags.Namespace = &kubeContext, &namespace
	config, err := flags.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	kube, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	dynamic, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &rotationStore{kube: kube, dynamic: dynamic, namespace: namespace, release: radiusReleaseName, now: time.Now}, nil
}

func (s *rotationStore) checkOrphanedCertificateSources(ctx context.Context) error {
	check := func(metadata metav1.Object) error {
		if metadata.GetAnnotations()[rotationRevision] != "" || metadata.GetAnnotations()[rotationJournalUID] != "" {
			return errors.New("managed Radius trust is missing its public journal; restore the journal before changing the release")
		}
		return nil
	}
	internal, err := s.kube.CoreV1().ConfigMaps(s.namespace).Get(ctx, "radius-mtls-ca", metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err == nil {
		if err := check(internal); err != nil {
			return err
		}
	}
	aggregation, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err == nil {
		return check(aggregation)
	}
	return nil
}

func (i *Impl) guardCertificateRotation(ctx context.Context, kubeContext, namespace, operation string, target *chart.Chart) error {
	store, err := i.rotationStoreForContext(kubeContext, namespace)
	if err != nil {
		return fmt.Errorf("cannot check Radius CA rotation before %s: %w", operation, err)
	}
	_, initial, initialErr := readInitialCertificateJournal(ctx, store)
	if initialErr != nil && !apierrors.IsNotFound(initialErr) {
		return initialErr
	}
	if initialErr == nil && initial.Phase != "complete" && operation != "install" {
		return errors.New("finish the initial certificate installation before changing the release")
	}
	if apierrors.IsNotFound(initialErr) {
		namespace, err := store.kube.CoreV1().Namespaces().Get(ctx, store.namespace, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if err == nil && namespace.Annotations[initialCertificateIntent] != "" && operation != "install" {
			return errors.New("recover the namespace's initial certificate intent before changing the release")
		}
	}
	_, found, bootstrapErr := store.bootstrapIntent(ctx)
	if bootstrapErr != nil {
		return bootstrapErr
	}
	if found {
		return errors.New("legacy aggregation bootstrap requires explicit recovery with the matching pre-token release before migration")
	}
	api, apiErr := store.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if apiErr != nil && !apierrors.IsNotFound(apiErr) {
		return fmt.Errorf("cannot inspect Radius APIService before %s: %w", operation, apiErr)
	}
	journal, err := store.kube.CoreV1().ConfigMaps(namespace).Get(ctx, rotationJournalName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if api != nil && (api.GetAnnotations()[rotationRevision] != "" || api.GetAnnotations()[rotationJournalUID] != "") {
			return errors.New("managed Radius trust is missing its public journal; restore the journal before changing the release")
		}
		return store.checkOrphanedCertificateSources(ctx)
	}

	if err != nil {
		return err
	}
	state, err := decodeCertificateLifecycle(journal, namespace, store.release, store.now())
	if err != nil {
		return err
	}
	if state.Phase != rotationComplete {
		return fmt.Errorf("CA rotation %s is unfinished in phase %s; resume it before %s", state.ID, state.Phase, operation)
	}
	if state.Execution != nil && state.Execution.FinalReleaseVersion == 0 {
		return errors.New("CA trust is retired but its final Helm reconciliation is unfinished; resume the lifecycle first")
	}
	if state.APIServiceUID != "" && (api == nil || api.GetUID() != state.APIServiceUID) {
		return errors.New("completed CA rotation belongs to a missing or replaced APIService; do not adopt it into a new installation")
	}
	if err := store.checkCertificateLifecycleRecovery(ctx, journal); err != nil {
		return err
	}
	if operation == "rollback" {
		return errors.New("rollback is disabled after managed CA rotation: deploy a guarded forward upgrade that preserves current trust instead")
	}
	if target != nil && (target.Metadata == nil || target.Metadata.Annotations["radius.io/ca-rotation-protocol"] != "4") {
		return errors.New("the target chart lacks the managed CA rotation protocol and could restore retired trust")
	}
	return nil
}
