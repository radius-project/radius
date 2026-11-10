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

func (i *Impl) guardCertificateRotation(ctx context.Context, kubeContext, namespace, operation string, target *chart.Chart) error {
	store, err := i.rotationStoreForContext(kubeContext, namespace)
	if err != nil {
		return fmt.Errorf("cannot check Radius CA rotation before %s: %w", operation, err)
	}
	if err := i.checkCompletedProviderJournal(ctx, store); err != nil {
		return err
	}
	bootstrap, found, bootstrapErr := store.bootstrapIntent(ctx)
	if bootstrapErr != nil {
		return bootstrapErr
	}
	if found && bootstrap.Phase != bootstrapComplete {
		if operation == "install" {
			// Install performs the stricter chart/settings/issuer identity check
			// before entering its resumable bootstrap driver.
			return nil
		}
		return errors.New("aggregation bootstrap is unfinished; resume the matching install before changing the release")
	}
	api, apiErr := store.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if apiErr != nil && !apierrors.IsNotFound(apiErr) {
		return fmt.Errorf("cannot inspect Radius APIService before %s: %w", operation, apiErr)
	}
	journal, state, err := store.read(ctx)
	if apierrors.IsNotFound(err) {
		if api != nil && (api.GetAnnotations()[rotationRevision] != "" || api.GetAnnotations()[rotationJournalUID] != "") {
			return errors.New("managed Radius trust is missing its public journal; restore the journal before changing the release")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if state.Phase != rotationComplete {
		return fmt.Errorf("CA rotation %s is unfinished in phase %s; resume it before %s", state.ID, state.Phase, operation)
	}
	if api == nil || api.GetUID() != state.APIServiceUID ||
		api.GetAnnotations()[rotationJournalUID] != string(journal.UID) {
		return errors.New("completed CA rotation belongs to a missing or replaced APIService; do not adopt it into a new installation")
	}
	if err := store.checkAPIService(api); err != nil {
		return err
	}
	if operation == "rollback" {
		return errors.New("rollback is disabled after managed CA rotation: deploy a guarded forward upgrade that preserves current trust instead")
	}
	if target != nil && (target.Metadata == nil || target.Metadata.Annotations["radius.io/ca-rotation-protocol"] != "1") {
		return errors.New("the target chart lacks the managed CA rotation protocol and could restore retired trust")
	}
	return nil
}
