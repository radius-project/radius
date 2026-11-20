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

	"github.com/radius-project/radius/pkg/authz"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type certificateOffOperationKey struct{}

type certificateOffOperation struct {
	kubeContext string
	store       *rotationStore
	operation   *identityOperation
	namespace   types.UID
	postHelm    bool
	observe     func(context.Context) error
	identities  map[string]types.UID
}

func (i *Impl) certificateOffStore(kubeContext, namespace string) (*rotationStore, error) {
	if i.newRotationStore != nil {
		return i.newRotationStore(kubeContext, namespace)
	}
	kube, dynamicClient, err := i.ucpAPIServerTrustClients(kubeContext)
	if err != nil {
		return nil, err
	}
	return &rotationStore{kube: kube, dynamic: dynamicClient, namespace: namespace, release: radiusReleaseName, now: time.Now}, nil
}

func (s *rotationStore) excludeCertificateOffTransition(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, name := range []string{initialCertificateJournal, rotationJournalName, identityProviderJournal, "radius-aggregation-bootstrap"} {
		if _, err := s.kube.CoreV1().ConfigMaps(s.namespace).Get(ctx, name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			if err != nil {
				return err
			}
			return fmt.Errorf("authorization Off is not supported while %s records lifecycle intent; preserve the journal and public sources", name)
		}
	}
	namespace, err := s.kube.CoreV1().Namespaces().Get(ctx, s.namespace, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err == nil && (namespace.Annotations[initialCertificateIntent] != "" ||
		namespace.Annotations["radius.io/aggregation-bootstrap-intent"] != "") {
		return errors.New("authorization Off cannot bypass a certificate bootstrap namespace intent")
	}
	check := func(object metav1.Object) error {
		if object.GetAnnotations()[initialCertificateIntent] != "" ||
			object.GetAnnotations()[rotationJournalUID] != "" || object.GetAnnotations()[rotationRevision] != "" {
			return errors.New("authorization Off cannot mutate a journal-bound public source; restore its lifecycle journal")
		}
		return nil
	}
	source, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err == nil {
		if err := check(source); err != nil {
			return err
		}
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
	api, err := s.dynamic.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err == nil {
		if err := check(api); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (o *certificateOffOperation) check(ctx context.Context) error {
	if err := o.operation.check(ctx); err != nil {
		return err
	}
	ns, err := o.store.kube.CoreV1().Namespaces().Get(ctx, o.store.namespace, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if ns.UID != o.namespace || ns.DeletionTimestamp != nil {
		return errors.New("authorization Off operation belongs to another namespace instance")
	}
	return o.store.excludeCertificateOffTransition(ctx)
}

func checkCertificateOffOperation(ctx context.Context) error {
	if operation, ok := ctx.Value(certificateOffOperationKey{}).(*certificateOffOperation); ok {
		return operation.check(ctx)
	}
	return ctx.Err()
}

func (i *Impl) withCertificateOffOperation(ctx context.Context, kubeContext, namespace string, mode authz.Mode, run func(context.Context) error) error {
	if mode != authz.ModeOff {
		return run(ctx)
	}
	if operation, ok := ctx.Value(certificateOffOperationKey{}).(*certificateOffOperation); ok {
		if operation.store.namespace != namespace || operation.kubeContext != kubeContext {
			return errors.New("authorization Off operation cannot cross contexts or namespaces")
		}
		if err := operation.check(ctx); err != nil {
			return err
		}
		return run(ctx)
	}
	store, err := i.certificateOffStore(kubeContext, namespace)
	if err != nil {
		return err
	}
	if err := store.excludeCertificateOffTransition(ctx); err != nil {
		return err
	}
	ns, err := store.kube.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, apiErr := store.dynamic.Resource(apiServicesResource).Get(ctx, ucpAPIServiceName, metav1.GetOptions{}); !apierrors.IsNotFound(apiErr) {
			if apiErr != nil {
				return apiErr
			}
			return errors.New("APIService exists without its Radius namespace; restore the installation before changing trust")
		}
		// A fresh Off install has no trust to transition. Post-Helm cleanup
		// acquires its own scope if the new namespace contains leftover trust.
		return run(ctx)
	}
	if err != nil {
		return err
	}
	if ns.UID == "" || ns.DeletionTimestamp != nil {
		return errors.New("authorization Off requires a live namespace identity")
	}
	return withIdentityOperation(ctx, store.kube, namespace, func(lease *identityOperation) error {
		observe := i.observeUCPOffServing
		if observe == nil {
			observe = observeOffAggregationServing
		}
		operation := &certificateOffOperation{kubeContext: kubeContext, store: store, operation: lease, namespace: ns.UID, identities: map[string]types.UID{},
			observe: func(ctx context.Context) error { return observe(ctx, kubeContext, namespace, store.kube) }}
		if err := operation.check(ctx); err != nil {
			return err
		}
		ctx = context.WithValue(ctx, certificateOffOperationKey{}, operation)
		ctx = withUCPMutationGuard(ctx, func(ctx context.Context, verb string, target metav1.PartialObjectMetadata) (bool, error) {
			if err := operation.check(ctx); err != nil {
				return false, err
			}
			if target.Namespace != namespace && !(target.Kind == "APIService" && target.Namespace == "") {
				return false, errors.New("UCP mutation target is outside the operation namespace")
			}
			if verb != "create" && (target.UID == "" || target.ResourceVersion == "") {
				return false, errors.New("UCP mutation requires observed UID and resourceVersion")
			}
			if target.DeletionTimestamp != nil {
				return false, errors.New("UCP mutation target is terminating")
			}
			if verb != "create" {
				key := target.Kind + "/" + target.Name
				if uid, found := operation.identities[key]; found && uid != target.UID {
					return false, errors.New("UCP mutation target was replaced during the operation")
				}
				operation.identities[key] = target.UID
			}
			if target.Kind == "APIService" || target.Kind == "Deployment" {
				if target.Labels[helmManagedByLabel] != helmManagedByValue ||
					target.Annotations[helmReleaseNameAnnotation] != store.release ||
					target.Annotations[helmReleaseNamespaceAnnotation] != namespace {
					return false, errors.New("UCP mutation target is not owned by this Radius release")
				}
			}
			if target.Annotations[initialCertificateIntent] != "" || target.Annotations[rotationJournalUID] != "" ||
				target.Annotations[rotationRevision] != "" {
				return false, errors.New("UCP mutation target belongs to a certificate lifecycle")
			}
			if operation.postHelm {
				if err := operation.observe(ctx); err != nil {
					return false, err
				}
				if err := operation.check(ctx); err != nil {
					return false, err
				}
			}
			return false, ctx.Err()
		})
		if err := run(ctx); err != nil {
			return err
		}
		return operation.check(ctx)
	})
}

// ReconcileUCPOffAPIServerTrust runs the inherited Off writer only within the
// post-Helm operation scope. Direct calls cannot bypass its Lease or journal guards.
func (i *Impl) ReconcileUCPOffAPIServerTrust(ctx context.Context, kubeContext, namespace string, declared manifestObjectSet) error {
	operation, ok := ctx.Value(certificateOffOperationKey{}).(*certificateOffOperation)
	if !ok || !operation.postHelm || operation.store.namespace != namespace || operation.kubeContext != kubeContext {
		return errors.New("Off trust reconciliation requires the post-Helm operation scope")
	}
	if err := operation.check(ctx); err != nil {
		return err
	}
	secrets := operation.store.kube.CoreV1().Secrets(namespace)
	offSecret, err := secrets.Get(ctx, ucpOffCertificateSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	ctx = withUCPMutationGuard(ctx, func(ctx context.Context, _ string, _ metav1.PartialObjectMetadata) (bool, error) {
		current, err := secrets.Get(ctx, ucpOffCertificateSecretName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if offSecret.UID == "" || offSecret.ResourceVersion == "" || current.UID != offSecret.UID ||
			current.ResourceVersion != offSecret.ResourceVersion || current.DeletionTimestamp != nil {
			return false, errors.New("Off serving source changed before trust mutation")
		}
		return false, ctx.Err()
	})
	return i.reconcileUCPOffAPIServerTrust(ctx, kubeContext, namespace, declared)
}
