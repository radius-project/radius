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
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (i *Impl) uninstallRadiusRelease(ctx context.Context, namespace, kubeContext string, confirmed []UninstallLeftover, removed *bool) error {
	store, err := i.rotationStoreForContext(kubeContext, namespace)
	if err != nil {
		return err
	}
	removeRelease := func() error {
		if confirmed != nil {
			rel, err := NewHelmAction(i.Helm).getRelease(kubeContext, radiusReleaseName, namespace)
			if err != nil {
				return err
			}
			if rel != nil {
				return errors.New("Radius is installed; confirm a new uninstall")
			}
			return nil
		}
		if err := i.uninstallHelmRelease("Radius", radiusReleaseName, namespace, kubeContext); err != nil {
			return err
		}
		*removed = true
		return nil
	}
	journal, state, err := readInitialCertificateJournal(ctx, store)
	if apierrors.IsNotFound(err) {
		ns, err := store.kube.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			// Helm release records are namespaced. Do not race a new
			// installation by calling Helm without an operation Lease.
			return nil
		}
		if err != nil {
			return err
		}
		if err := store.excludeCertificateOffTransition(ctx); err != nil {
			return err
		}
		return withIdentityOperation(ctx, store.kube, namespace, func(operation *identityOperation) error {
			if err := store.excludeCertificateOffTransition(ctx); err != nil {
				return err
			}
			current, err := store.kube.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if ns.UID == "" || current.UID != ns.UID || current.DeletionTimestamp != nil {
				return errors.New("uninstall namespace was replaced or is terminating")
			}
			if err := operation.check(ctx); err != nil {
				return err
			}
			if err := removeRelease(); err != nil {
				return err
			}
			ctx = withUCPMutationGuard(ctx, func(ctx context.Context, verb string, target metav1.PartialObjectMetadata) (bool, error) {
				if err := operation.check(ctx); err != nil {
					return false, err
				}
				current, err := store.kube.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
				if err != nil {
					return false, err
				}
				if ns.UID == "" || current.UID != ns.UID || current.DeletionTimestamp != nil ||
					verb != "delete" || target.Namespace != namespace ||
					target.UID == "" || target.ResourceVersion == "" {
					return false, errors.New("uninstall trust cleanup lost its pinned namespace or source")
				}
				if err := store.excludeCertificateOffTransition(ctx); err != nil {
					return false, err
				}
				if confirmed != nil && !matchesConfirmed(confirmed, target.Kind, &target) {
					i.logInfo("Warning: retaining changed %s %s/%s; confirm again", target.Kind, target.Namespace, target.Name)
					return true, nil
				}
				return false, ctx.Err()
			})
			if confirmed == nil {
				return i.deleteUCPAPIServerLeftovers(ctx, kubeContext, store)
			}
			for _, l := range confirmed {
				for _, stage := range uninstallStages() {
					if l.Kind == stage.kind && l.Name == stage.name && l.resource == stage.resource {
						if err := i.deleteUnadoptedStagedObject(ctx, kubeContext, store.dynamic, stage, namespace, radiusReleaseName); err != nil {
							return err
						}
					}
				}
			}
			return i.deleteUCPOffLeftovers(ctx, kubeContext, store.kube, store.dynamic, namespace, confirmedIssued(ucpOffLeftoverSecrets, confirmed))
		})
	}
	if err != nil {
		return err
	}
	if state.Phase != "complete" {
		return errors.New("finish the initial certificate installation before uninstalling")
	}
	if confirmed != nil && !matchesConfirmed(confirmed, "ConfigMap", journal) {
		return errors.New("bootstrap uninstall inventory changed; confirm again")
	}
	b := &initialCertificateInstaller{impl: i, store: store, journal: journal, state: state}
	return withIdentityOperation(ctx, store.kube, namespace, func(operation *identityOperation) error {
		b.operation = operation
		if err := b.check(ctx); err != nil {
			return err
		}
		if err := removeRelease(); err != nil {
			return err
		}
		return b.retireBootstrap(ctx, confirmed)
	})
}

func (b *initialCertificateInstaller) visitBootstrapPins(ctx context.Context, visit func(string, metav1.Object, func(metav1.DeleteOptions) error) error) error {
	// Keep the journal until every cleanup succeeds, so an interrupted uninstall
	// can retry without accepting a foreign object as a new bootstrap.
	for _, plane := range b.planeNames() {
		if b.state.Mode == "selfSigned" {
			pin := b.state.Anchors[plane].Signer
			name := "radius-apiserver-ca"
			if plane == "internal" {
				name = "radius-ca"
			}
			if pin == nil || pin.Name != name {
				return errors.New("bootstrap signer is not the expected generated root")
			}
			if _, err := b.store.dynamic.Resource(rotationCertificateResource).Namespace(b.store.namespace).Get(ctx, name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
				if err != nil {
					return err
				}
				return fmt.Errorf("wait for the uninstalled Certificate %s to disappear before retiring its Secret", name)
			}
			client := b.store.kube.CoreV1().Secrets(b.store.namespace)
			secret, err := client.Get(ctx, name, metav1.GetOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			if err == nil {
				targeted, err := liveCertificateTargets(ctx, b.store.dynamic, b.store.namespace, name)
				if err != nil {
					return err
				}
				if targeted {
					return fmt.Errorf("preserving bootstrap signer %s: a live Certificate still targets it", name)
				}
				if err := validateBootstrapRenewal(secret, b.state.Anchors[plane], b.store.now()); err != nil {
					return fmt.Errorf("preserving changed or foreign bootstrap signing Secret %s: %w", name, err)
				}
				if err := visit("Secret", secret, func(options metav1.DeleteOptions) error {
					return client.Delete(ctx, name, options)
				}); err != nil {
					return err
				}
			}
		}
		pin := b.state.Sources[plane]
		var object metav1.Object
		var bundle []byte
		var remove func(metav1.DeleteOptions) error
		if plane == "aggregation" {
			client := b.store.kube.CoreV1().Secrets(b.store.namespace)
			source, err := client.Get(ctx, pin.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			if err := publicAggregationSource(source, b.store.namespace); err != nil {
				return err
			}
			object, bundle = source, source.Data[caCertificateKey]
			remove = func(options metav1.DeleteOptions) error { return client.Delete(ctx, pin.Name, options) }
		} else {
			client := b.store.kube.CoreV1().ConfigMaps(b.store.namespace)
			source, err := client.Get(ctx, pin.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			if len(source.Data) != 1 || len(source.BinaryData) != 0 {
				return errors.New("preserving bootstrap ConfigMap with non-public data")
			}
			object, bundle = source, []byte(source.Data[caCertificateKey])
			remove = func(options metav1.DeleteOptions) error { return client.Delete(ctx, pin.Name, options) }
		}
		annotations := object.GetAnnotations()
		if object.GetUID() != pin.UID || len(object.GetOwnerReferences()) != 0 ||
			object.GetLabels()[helmManagedByLabel] != helmManagedByValue ||
			annotations[helmReleaseNameAnnotation] != b.store.release ||
			annotations[helmReleaseNamespaceAnnotation] != b.store.namespace ||
			annotations[initialCertificateIntent] != string(b.journal.UID) ||
			annotations[rotationJournalUID] != "" || annotations[rotationRevision] != "" ||
			!bytes.Equal(bundle, pin.Bundle) {
			return fmt.Errorf("preserving changed or foreign bootstrap public source %s", pin.Name)
		}
		kind := "Secret"
		if plane != "aggregation" {
			kind = "ConfigMap"
		}
		if err := visit(kind, object, remove); err != nil {
			return err
		}
	}
	return nil
}

func (b *initialCertificateInstaller) retireBootstrap(ctx context.Context, confirmed []UninstallLeftover) error {
	if err := b.visitBootstrapPins(ctx, func(kind string, obj metav1.Object, remove func(metav1.DeleteOptions) error) error {
		if confirmed != nil && !matchesConfirmed(confirmed, kind, obj) {
			return fmt.Errorf("preserving changed or unconfirmed bootstrap %s %s; confirm again", kind, obj.GetName())
		}
		return b.deleteBootstrapObject(ctx, obj, remove)
	}); err != nil {
		return err
	}
	// The signer and public source have stronger journal-specific checks above.
	// Never let generic ownership metadata authorize their deletion on retry.
	orphans := b.bootstrapOrphans()
	guarded := withUCPMutationGuard(ctx, func(ctx context.Context, _ string, target metav1.PartialObjectMetadata) (bool, error) {
		if confirmed != nil && !matchesConfirmed(confirmed, target.Kind, &target) {
			return false, errors.New("bootstrap leaf inventory changed; confirm again")
		}
		return false, b.check(ctx)
	})
	if err := b.impl.deleteUCPOffLeftovers(guarded, "", b.store.kube, b.store.dynamic, b.store.namespace, confirmedIssued(orphans, confirmed)); err != nil {
		return err
	}
	// The shared deleter retains replacements on conflict. Keep the recovery journal if eligible leaves remain.
	remaining, err := ucpOffLeftovers(ctx, b.store.kube, b.store.dynamic, b.store.namespace, orphans)
	if err != nil {
		return err
	}
	if hasUCPOffLeftover(remaining) {
		return errors.New("bootstrap leaves changed during cleanup; retry uninstall")
	}
	namespace, err := b.store.kube.CoreV1().Namespaces().Get(ctx, b.store.namespace, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if raw := namespace.Annotations[initialCertificateIntent]; raw != "" {
		intent, err := decodeInitialCertificateState(raw, b.store, false)
		if err != nil || intent.ID != b.state.ID || intent.IntentSHA256 != b.state.IntentSHA256 ||
			intent.ChartSHA256 != b.state.ChartSHA256 || intent.ValuesSHA256 != b.state.ValuesSHA256 ||
			intent.Mode != b.state.Mode || intent.Provider != b.state.Provider {
			return errors.New("preserving foreign namespace bootstrap intent")
		}
		patch, err := json.Marshal([]map[string]any{
			{"op": "test", "path": "/metadata/uid", "value": b.state.NamespaceUID},
			{"op": "test", "path": "/metadata/resourceVersion", "value": namespace.ResourceVersion},
			{"op": "test", "path": "/metadata/annotations/radius.io~1certificate-bootstrap", "value": raw},
			{"op": "remove", "path": "/metadata/annotations/radius.io~1certificate-bootstrap"},
		})
		if err != nil {
			return err
		}
		if err := b.check(ctx); err != nil {
			return err
		}
		if _, err := b.store.kube.CoreV1().Namespaces().Patch(ctx, namespace.Name, types.JSONPatchType, patch, metav1.PatchOptions{FieldManager: FieldManager}); err != nil {
			return err
		}
	}
	return b.deleteBootstrapObject(ctx, b.journal, func(options metav1.DeleteOptions) error {
		return b.store.kube.CoreV1().ConfigMaps(b.store.namespace).Delete(ctx, b.journal.Name, options)
	})
}

func (b *initialCertificateInstaller) bootstrapOrphans() []ucpOffLeftoverSecret {
	var orphans []ucpOffLeftoverSecret
	for _, entry := range ucpOffLeftoverSecrets {
		if entry.certificate == "" || entry.secret == "radius-ca" || entry.secret == ucpAPIServerCASecretName {
			continue
		}
		protected := false
		for _, anchor := range b.state.Anchors {
			if anchor.Signer != nil && anchor.Signer.Name == entry.secret {
				protected = true
			}
		}
		if !protected {
			orphans = append(orphans, entry)
		}
	}
	return orphans
}

func (b *initialCertificateInstaller) deleteBootstrapObject(ctx context.Context, object metav1.Object, remove func(metav1.DeleteOptions) error) error {
	uid, version := object.GetUID(), object.GetResourceVersion()
	if uid == "" || version == "" || object.GetDeletionTimestamp() != nil {
		return fmt.Errorf("bootstrap cleanup requires a live identity for %s", object.GetName())
	}
	if err := b.check(ctx); err != nil {
		return err
	}
	return remove(metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &version}})
}
