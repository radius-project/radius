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
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"

	"github.com/radius-project/radius/pkg/authz/mtls"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// This snapshot is a public identity pin, NOT a lock. CA/provider operations
// must share exclusion before check-then-write can authorize provider changes.
type aggregationTransitionSnapshot struct {
	APIServiceUID types.UID     `json:"apiServiceUID"`
	JournalUID    types.UID     `json:"journalUID,omitempty"`
	Revision      string        `json:"revision,omitempty"`
	TrustSHA256   string        `json:"trustSHA256"`
	SignerSHA256  string        `json:"signerSHA256,omitempty"`
	Scope         rotationScope `json:"scope,omitempty"`
	Complete      bool          `json:"complete"`
}

func (s *rotationStore) aggregationTransitionSnapshot(ctx context.Context) (aggregationTransitionSnapshot, error) {
	bootstrap, found, err := s.bootstrapIntent(ctx)
	if err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	if found && bootstrap.Phase != bootstrapComplete {
		return aggregationTransitionSnapshot{}, errors.New("complete aggregation bootstrap before changing provider")
	}
	api, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	if err := s.checkAPIService(api); err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	public, err := apiServiceBundle(api)
	if err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	digest, roots, err := mtls.TrustBundleDigest(public, s.now())
	if err != nil || len(roots) != 1 {
		return aggregationTransitionSnapshot{}, errors.New("provider transition requires one valid aggregation CA")
	}
	journal, state, err := s.read(ctx)
	if apierrors.IsNotFound(err) {
		if api.GetAnnotations()[rotationJournalUID] != "" || api.GetAnnotations()[rotationRevision] != "" {
			return aggregationTransitionSnapshot{}, errors.New("managed aggregation trust is missing its journal")
		}
		return aggregationTransitionSnapshot{APIServiceUID: api.GetUID(), TrustSHA256: digest}, nil
	}
	if err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	if state.Phase != rotationComplete || state.Plan == nil {
		return aggregationTransitionSnapshot{}, errors.New("complete the CA rotation before changing provider")
	}
	if journal.UID == "" || state.APIServiceUID != api.GetUID() || api.GetAnnotations()[rotationJournalUID] != string(journal.UID) ||
		api.GetAnnotations()[rotationRevision] != state.revision() || !bytes.Equal(public, state.NewCA) {
		return aggregationTransitionSnapshot{}, errors.New("aggregation identity or trust changed outside its completed journal")
	}
	var issuer *unstructured.Unstructured
	if state.Plan.ExternalIssuer != nil {
		// A provider snapshot pins public identity; it neither issues a request
		// nor claims that the target provider's policy or workloads are ready.
		issuer, err = s.externalRotationIssuer(ctx, state.Plan)
		if err != nil {
			return aggregationTransitionSnapshot{}, err
		}
	} else {
		if err := s.checkPlannedSigner(ctx, state); err != nil {
			return aggregationTransitionSnapshot{}, err
		}
	}
	workload, err := state.Plan.aggregationWorkload()
	if err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	deployment, err := s.kube.AppsV1().Deployments(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	if deployment.UID != workload.DeploymentUID || !rotationOwned(deployment, s.namespace, s.release) {
		return aggregationTransitionSnapshot{}, errors.New("aggregation deployment identity changed")
	}
	if err := rotationAggregationProjection(deployment.Spec.Template.Spec, state.Plan); err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	service, err := s.kube.CoreV1().Services(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	if service.UID != state.Plan.ServiceUID || !rotationOwned(service, s.namespace, s.release) ||
		!maps.Equal(service.Spec.Selector, state.Plan.ServiceSelector) {
		return aggregationTransitionSnapshot{}, errors.New("aggregation Service identity or completed routing changed")
	}
	trust, err := s.kube.CoreV1().ConfigMaps(s.namespace).Get(ctx, state.Plan.TargetTrustMap, metav1.GetOptions{})
	if err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	trustDigest, _, err := mtls.TrustBundleDigest([]byte(trust.Data["ca.crt"]), s.now())
	if err != nil || trustDigest != digest || trust.UID == "" || trust.DeletionTimestamp != nil ||
		trust.Immutable == nil || !*trust.Immutable || trust.Labels[rotationOwner] != s.release {
		return aggregationTransitionSnapshot{}, errors.New("aggregation immutable trust identity changed")
	}
	if issuer == nil {
		issuer, err = s.dynamic.Resource(rotationIssuerResource).Namespace(s.namespace).Get(ctx, state.Plan.Issuer, metav1.GetOptions{})
		if err != nil {
			return aggregationTransitionSnapshot{}, err
		}
		secretName, _, err := unstructured.NestedString(issuer.Object, "spec", "ca", "secretName")
		if err != nil || secretName != state.Plan.SigningSecret || issuer.GetUID() == "" || issuer.GetDeletionTimestamp() != nil ||
			issuer.GetLabels()[rotationOwner] != s.release || issuer.GetLabels()[rotationTransaction] != state.ID {
			return aggregationTransitionSnapshot{}, errors.New("aggregation issuer identity or configuration changed")
		}
	}
	certificate, err := s.dynamic.Resource(rotationCertificateResource).Namespace(s.namespace).Get(ctx, state.Plan.ServingSecret, metav1.GetOptions{})
	if err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	serving, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, state.Plan.ServingSecret, metav1.GetOptions{})
	if err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	if _, err := rotationLeafDigest(serving, state.NewCA, s.namespace, "ucp", false, s.now()); err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	if certificate.GetUID() == "" {
		return aggregationTransitionSnapshot{}, errors.New("aggregation Certificate has no identity")
	}
	if err := checkAggregationCertificate(certificate, state, s.namespace); err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	// Exclude leaf bytes and status/resourceVersion so ordinary renewal does not
	// invalidate a transition. Include resource UIDs and all public issuer/spec intent.
	config := struct {
		Mode            string
		SigningUID      types.UID
		IssuerUID       types.UID
		CertificateUID  types.UID
		ServingUID      types.UID
		Trust           string
		DeploymentUID   types.UID
		ServiceUID      types.UID
		TrustUID        types.UID
		IssuerSpec      any
		CertificateSpec any
		ExternalIssuer  *rotationExternalIssuer `json:"ExternalIssuer,omitempty"`
	}{state.Plan.TargetMode, state.Plan.SigningUID, issuer.GetUID(), certificate.GetUID(), serving.UID,
		state.Plan.TargetTrustMap, deployment.UID, service.UID, trust.UID, issuer.Object["spec"], certificate.Object["spec"], state.Plan.ExternalIssuer}
	encoded, err := json.Marshal(config)
	if err != nil {
		return aggregationTransitionSnapshot{}, err
	}
	sum := sha256.Sum256(encoded)
	result := aggregationTransitionSnapshot{APIServiceUID: api.GetUID(), JournalUID: journal.UID,
		Revision: state.revision(), TrustSHA256: digest, SignerSHA256: hex.EncodeToString(sum[:]),
		Scope: state.Plan.effectiveScope(), Complete: true}
	after, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if err != nil || after.GetUID() != api.GetUID() || after.GetResourceVersion() != api.GetResourceVersion() {
		return aggregationTransitionSnapshot{}, errors.New("aggregation APIService changed during provider snapshot")
	}
	current, currentState, err := s.read(ctx)
	if err != nil || current.UID != journal.UID || current.ResourceVersion != journal.ResourceVersion || currentState.ID != state.ID {
		return aggregationTransitionSnapshot{}, errors.New("CA journal changed during provider snapshot")
	}
	return result, nil
}

func (s *rotationStore) checkAggregationTransitionSnapshot(ctx context.Context, expected aggregationTransitionSnapshot) error {
	if !expected.Complete || expected.JournalUID == "" || expected.SignerSHA256 == "" {
		return errors.New("provider transition requires completed, explicitly managed aggregation state")
	}
	current, err := s.aggregationTransitionSnapshot(ctx)
	if err != nil {
		return err
	}
	if current != expected {
		return errors.New("aggregation state changed during provider transition")
	}
	return nil
}
