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
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/radius-project/radius/pkg/authz/mtls"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// This public configuration pin is neither a lock nor serving/readiness evidence.
// Provider operations must hold the shared Lease and observe their own workloads.
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
	var result aggregationTransitionSnapshot
	_, initial, err := readInitialCertificateJournal(ctx, s)
	if err != nil && !apierrors.IsNotFound(err) {
		return result, err
	}
	if err == nil && initial.Phase != "complete" {
		return result, errors.New("complete initial certificate bootstrap before changing provider")
	}
	namespace, err := s.kube.CoreV1().Namespaces().Get(ctx, s.namespace, metav1.GetOptions{})
	if err != nil {
		return result, err
	}
	if namespace.Annotations[initialCertificateIntent] != "" && initial.Phase != "complete" {
		return result, errors.New("recover the namespace's initial certificate intent before changing provider")
	}
	if _, found, err := s.bootstrapIntent(ctx); err != nil {
		return result, err
	} else if found {
		return result, errors.New("legacy aggregation bootstrap requires recovery with the matching pre-token release")
	}
	api, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if err != nil {
		return result, err
	}
	if err := s.checkLifecycleAPIService(api); err != nil {
		return result, err
	}
	public, err := apiServiceBundle(api)
	if err != nil {
		return result, err
	}
	digest, roots, err := mtls.TrustBundleDigest(public, s.now())
	if err != nil || len(roots) != 1 {
		return result, errors.New("provider transition requires exactly one valid aggregation CA")
	}
	result.APIServiceUID, result.TrustSHA256 = api.GetUID(), digest
	journal, err := s.kube.CoreV1().ConfigMaps(s.namespace).Get(ctx, rotationJournalName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if api.GetAnnotations()[rotationJournalUID] != "" || api.GetAnnotations()[rotationRevision] != "" {
			return result, errors.New("managed aggregation trust is missing its journal")
		}
		// The provider owns baseline initialization. Do not invent a completed
		// rotation or import legacy shared-certificate evidence into its plan.
		return result, ctx.Err()
	}
	if err != nil {
		return result, err
	}
	state, err := decodeCertificateLifecycle(journal, s.namespace, s.release, s.now())
	if err != nil {
		return result, err
	}
	if state.Phase != rotationComplete || state.Execution != nil && state.Execution.FinalReleaseVersion == 0 {
		return result, errors.New("complete the CA rotation before changing provider")
	}
	if err := s.checkCertificateLifecycleRecovery(ctx, journal); err != nil {
		return result, err
	}
	plane := state.Planes["aggregation"]
	if state.APIServiceUID != "" && state.APIServiceUID != api.GetUID() {
		return result, errors.New("completed aggregation APIService identity changed")
	}
	if err := exactCertificateSet(public, plane.NewCA, s.now()); err != nil {
		return result, err
	}
	if api.GetAnnotations()[injectCAFromSecretAnnotation] != s.namespace+"/"+plane.Source.Name ||
		api.GetAnnotations()["cert-manager.io/inject-ca-from"] != "" ||
		api.GetAnnotations()["cert-manager.io/inject-apiserver-ca"] != "" {
		return result, errors.New("aggregation APIService no longer uses its permanent public source")
	}
	issuer := plane.TargetIssuer
	issuer.ResourceVersion = ""
	var signer *certificateObjectPin
	if plane.Signer != nil {
		copy := *plane.Signer
		copy.ResourceVersion = ""
		signer = &copy
	}
	encoded, err := json.Marshal(struct {
		Issuer certificateIssuerSnapshot
		Signer *certificateObjectPin
		Source types.UID
		Mode   string
	}{issuer, signer, plane.Source.UID, state.Mode})
	if err != nil {
		return result, err
	}
	result.JournalUID, result.Revision = journal.UID, state.ID+"/complete"
	result.SignerSHA256 = fmt.Sprintf("%x", sha256.Sum256(encoded))
	result.Scope, result.Complete = rotationAggregationOnly, true
	if state.Provider == "radius" {
		result.Scope = rotationComponentsAndAggregation
	}
	current, err := s.kube.CoreV1().ConfigMaps(s.namespace).Get(ctx, journal.Name, metav1.GetOptions{})
	if err != nil {
		return result, err
	}
	after, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	if err != nil {
		return result, err
	}
	if current.UID != journal.UID || current.ResourceVersion != journal.ResourceVersion ||
		!reflect.DeepEqual(current.Data, journal.Data) ||
		after.GetUID() != api.GetUID() || after.GetResourceVersion() != api.GetResourceVersion() {
		return result, errors.New("aggregation state changed during provider snapshot")
	}
	return result, ctx.Err()
}

func (s *rotationStore) checkLifecycleAPIService(api *unstructured.Unstructured) error {
	if api.GetUID() == "" || api.GetResourceVersion() == "" || api.GetDeletionTimestamp() != nil ||
		len(api.GetOwnerReferences()) != 0 || !rotationOwned(api, s.namespace, s.release) {
		return errors.New("aggregation APIService identity or ownership changed")
	}
	namespace, _, err := unstructured.NestedString(api.Object, "spec", "service", "namespace")
	if err != nil || namespace != s.namespace {
		return errors.New("aggregation APIService points to another namespace")
	}
	name, _, err := unstructured.NestedString(api.Object, "spec", "service", "name")
	if err != nil || name != "ucp" {
		return errors.New("aggregation APIService points to another service")
	}
	insecure, _, err := unstructured.NestedBool(api.Object, "spec", "insecureSkipTLSVerify")
	if err != nil || insecure {
		return errors.New("aggregation APIService must verify serving certificates")
	}
	return nil
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
