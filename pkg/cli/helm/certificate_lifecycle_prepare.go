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
	"crypto/sha256"
	"encoding/pem"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
)

// planCertificateLifecycle is read-only. Planned resources and unresolved roots
// are not observations, and this result does not authorize acquiring a Lease.
func (s *rotationStore) planCertificateLifecycle(ctx context.Context, installed *releasev1.Release, selected *chart.Chart,
	target *certificateTarget, scope string) (certificateLifecycle, error) {
	var state certificateLifecycle
	if installed == nil || installed.Chart == nil || installed.Version < 1 || selected == nil || target == nil {
		return state, errors.New("certificate planning requires the installed release and selected target")
	}
	effective, err := valuesutil.CoalesceValues(installed.Chart, copyValues(installed.Config))
	if err != nil {
		return state, err
	}
	baseline, err := certificateProfilePlan(installed.Chart, effective, s.namespace)
	if err != nil {
		return state, err
	}
	if baseline.Provider != target.Provider {
		return state, errors.New("CA replacement cannot also change the TLS provider")
	}
	if baseline.TrustDomain != target.TrustDomain || !slices.Equal(baseline.Receivers, target.Receivers) ||
		resolveSPIFFESettings(nil, baseline.Values) != resolveSPIFFESettings(nil, target.Values) {
		return state, errors.New("CA replacement cannot also change the receiver inventory, trust domain or CSI driver")
	}
	if scope != "" && baseline.Mode != target.Mode {
		return state, errors.New("a certificate mode change must include every enabled plane")
	}
	affected, err := certificateRotationPlanes(target.Provider, scope)
	if err != nil {
		return state, err
	}
	if target.Mode == "issuerRef" && slices.Contains(affected, "internal") {
		return state, errors.New("internal issuerRef replacement requires a verified staged CSI issuer configuration; this executor does not switch the separately managed driver")
	}
	snapshots, err := s.snapshotCertificateInstallation(ctx, baseline)
	if err != nil {
		return state, err
	}
	namespace, err := s.kube.CoreV1().Namespaces().Get(ctx, s.namespace, metav1.GetOptions{})
	if err != nil {
		return state, err
	}
	if namespace.UID == "" || namespace.DeletionTimestamp != nil {
		return state, errors.New("certificate planning requires a live namespace identity")
	}
	state = certificateLifecycle{SchemaVersion: 4, ID: uuid.NewString(), Namespace: s.namespace, Release: s.release,
		Provider: target.Provider, Mode: target.Mode, Phase: certificatePrepare, CreatedAt: s.now(),
		Planes: map[string]certificateLifecyclePlane{}, Execution: &certificateLifecycleExecution{
			NamespaceUID: namespace.UID, BaselineVersion: installed.Version,
		}}
	state.Execution.ChartSHA256, err = bootstrapDigest(selected)
	if err != nil {
		return state, err
	}
	state.Execution.BaselineValuesSHA256, err = bootstrapDigest(installed.Config)
	if err != nil {
		return state, err
	}
	state.Execution.TargetValuesSHA256, err = bootstrapDigest(target.Values)
	if err != nil {
		return state, err
	}
	for name, snapshot := range snapshots {
		selection, found := target.Planes[name]
		if !found {
			return state, errors.New("certificate target dropped an enabled trust plane")
		}
		plane := certificateLifecyclePlane{Affected: slices.Contains(affected, name), Previous: &snapshot,
			OldCA: bytes.Clone(snapshot.Source.Bundle), Source: snapshot.Source}
		if !plane.Affected {
			plane.NewCA, plane.Issuer = bytes.Clone(plane.OldCA), snapshot.Issuer.Reference
			plane.TargetIssuer, plane.Signer = snapshot.Issuer, snapshot.Signer
		} else {
			plane.Issuer, plane.NewCA = selection.Issuer, bytes.Clone(selection.CA)
			if target.Mode == "issuerRef" {
				if plane.Issuer == snapshot.Issuer.Reference {
					return state, errors.New("issuerRef migration requires a distinct staged target issuer")
				}
				plane.TargetIssuer, err = s.snapshotCertificateIssuer(ctx, plane.Issuer)
				if err != nil {
					return state, err
				}
			} else if target.SigningUID != "" {
				plane.Signer = &certificateObjectPin{Name: target.SigningSecret, UID: target.SigningUID,
					ResourceVersion: target.SigningVersion, SHA256: target.SigningDERHash}
			}
		}
		state.Planes[name] = plane
	}
	if slices.Contains(affected, "aggregation") {
		injectors, err := inspectCAInjectorPrerequisites(ctx, s.kube, s.namespace)
		if err != nil {
			return state, err
		}
		state.Execution.CAInjectorSHA256, err = bootstrapDigest(injectors)
		if err != nil {
			return state, err
		}
		api, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
		if err != nil {
			return state, err
		}
		if err := s.checkLifecycleAPIService(api); err != nil {
			return state, err
		}
		state.APIServiceUID = api.GetUID()
	}
	if _, enabled := state.Planes["aggregation"]; enabled {
		leaf, err := s.dynamic.Resource(rotationCertificateResource).Namespace(s.namespace).Get(ctx, "ucp-cert", metav1.GetOptions{})
		if err != nil {
			return state, err
		}
		if !rotationOwned(leaf, s.namespace, s.release) || leaf.GetDeletionTimestamp() != nil || len(leaf.GetOwnerReferences()) != 0 {
			return state, errors.New("aggregation leaf Certificate has foreign ownership")
		}
		state.Execution.PreviousServingSecret, _, err = unstructured.NestedString(leaf.Object, "spec", "secretName")
		if err != nil {
			return state, err
		}
		state.Execution.ServingSecret = state.Execution.PreviousServingSecret
		if state.Planes["aggregation"].Affected {
			state.Execution.ServingSecret = "radius-aggregation-" + state.ID
			if _, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, state.Execution.ServingSecret, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
				if err != nil {
					return state, err
				}
				return state, errors.New("planned aggregation serving Secret already exists; do not adopt or overwrite it")
			}
		} else {
			pin, err := bootstrapResourcePin(leaf)
			if err != nil {
				return state, err
			}
			state.Execution.UnchangedAggregationLeaf = &pin
		}
	}
	state.Execution.Resources, err = s.planCertificateResources(ctx, selected, target, state)
	if err != nil {
		return state, err
	}
	return state, state.validate(s.now())
}

func (w *certificateExecutionWriter) prepareTarget(ctx context.Context, target *certificateTarget, imports *certificatePlan, impl *Impl) error {
	if w.observeLoaded == nil {
		return errCertificateObserverUnavailable
	}
	if w.state.Phase != certificatePrepare || w.journal == nil || target == nil ||
		target.Mode != w.state.Mode || target.Provider != w.state.Provider {
		return errors.New("target preparation requires the matching durable preparation intent")
	}
	digest, err := bootstrapDigest(target.Values)
	if err != nil || digest != w.state.Execution.TargetValuesSHA256 {
		return errors.New("target preparation values changed")
	}
	if imports != nil {
		if impl == nil {
			return errors.New("private imports require the prepared certificate implementation")
		}
		imports.beforeWrite, imports.requiresLease = w.check, true
		if err := impl.applyCertificatePlan(ctx, imports); err != nil {
			return err
		}
	}
	for index, intent := range w.state.Execution.Resources {
		if intent.Kind == "Issuer" && intent.Spec["ca"] != nil {
			continue
		}
		if err := w.ensureResource(ctx, index); err != nil {
			return err
		}
	}
	for _, name := range []string{"internal", "aggregation"} {
		plane, found := w.state.Planes[name]
		if !found || !plane.Affected || w.state.Mode == "issuerRef" || plane.Signer != nil {
			continue
		}
		secretName := target.SigningSecret
		if w.state.Mode == "selfSigned" {
			secretName = lifecycleRootName(w.state, name)
		}
		if err := w.observePreparedSigner(ctx, name, secretName, target); err != nil {
			return err
		}
	}
	return w.check(ctx)
}

func (w *certificateExecutionWriter) observePreparedSigner(ctx context.Context, planeName, secretName string, target *certificateTarget) error {
	var secret *corev1.Secret
	err := wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, effectiveTimeout(0), true, func(ctx context.Context) (bool, error) {
		if err := w.check(ctx); err != nil {
			return false, err
		}
		var err error
		secret, err = w.store.kube.CoreV1().Secrets(w.store.namespace).Get(ctx, secretName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) && w.state.Mode == "selfSigned" {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if secret.Type != corev1.SecretTypeTLS || secret.UID == "" || secret.ResourceVersion == "" || secret.DeletionTimestamp != nil {
			return false, errors.New("prepared signer has an invalid TLS Secret identity")
		}
		if w.state.Mode == "selfSigned" {
			for _, intent := range w.state.Execution.Resources {
				if intent.Kind == "Certificate" && intent.Name == secretName && intent.Plane == planeName && intent.Observed != nil {
					return certificateRootIssued(ctx, w.store, secretName, *intent.Observed, secret)
				}
			}
			return false, errors.New("selfSigned signer has no observed root Certificate")
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("observe prepared %s signer: %w", planeName, err)
	}
	ca, err := validateSigningCA(secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey], w.store.now())
	if err != nil {
		return err
	}
	chain, err := parseCertificateBundlePEM(secret.Data[corev1.TLSCertKey])
	if err != nil {
		return err
	}
	if err := validateCAProfiles(chain, target, w.store.namespace, w.store.now()); err != nil {
		return err
	}
	next, err := cloneCertificateLifecycle(w.state)
	if err != nil {
		return err
	}
	plane := next.Planes[planeName]
	bundle := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})
	if len(plane.NewCA) != 0 && !bytes.Equal(plane.NewCA, bundle) {
		return errors.New("prepared signer differs from the intended dedicated public CA")
	}
	plane.NewCA = bundle
	plane.Signer = &certificateObjectPin{Name: secretName, UID: secret.UID, ResourceVersion: secret.ResourceVersion,
		SHA256: fmt.Sprintf("%x", sha256.Sum256(ca.Raw))}
	next.Planes[planeName] = plane
	return w.save(ctx, next)
}
