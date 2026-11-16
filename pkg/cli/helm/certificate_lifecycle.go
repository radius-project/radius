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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var errCertificateObserverUnavailable = errors.New("CA lifecycle blocked before mutation: production observation coverage and authorized access are unavailable for every required outbound authority and aggregation trust consumer; component ServiceAccount token minting, token extraction, API bearer forwarding and anonymous proof routes are not permitted")

// certificateLifecycle is the token/CSI journal. Legacy journals cannot be
// reinterpreted: their CertificateUIDs do not identify pod-local CSI material.
type certificateLifecycle struct {
	SchemaVersion int                                  `json:"schemaVersion"`
	ID            string                               `json:"id"`
	Namespace     string                               `json:"namespace"`
	Release       string                               `json:"release"`
	Provider      string                               `json:"provider"`
	Mode          string                               `json:"mode"`
	Phase         rotationPhase                        `json:"phase"`
	CreatedAt     time.Time                            `json:"createdAt"`
	APIServiceUID types.UID                            `json:"apiServiceUID,omitempty"`
	Planes        map[string]certificateLifecyclePlane `json:"planes"`
	Consumers     []certificateConsumerPin             `json:"consumers,omitempty"`
	Execution     *certificateLifecycleExecution       `json:"execution,omitempty"`
}

type certificateLifecyclePlane struct {
	Affected     bool                       `json:"affected"`
	OldCA        []byte                     `json:"oldCA"`
	NewCA        []byte                     `json:"newCA"`
	Issuer       certificateIssuerReference `json:"issuer"`
	Signer       *certificateObjectPin      `json:"signer,omitempty"`
	Source       certificateTrustProjection `json:"source"`
	Previous     *certificatePlaneSnapshot  `json:"previous"`
	TargetIssuer certificateIssuerSnapshot  `json:"targetIssuer"`
}

type certificateObjectPin struct {
	Name            string    `json:"name"`
	UID             types.UID `json:"uid"`
	ResourceVersion string    `json:"resourceVersion"`
	SHA256          string    `json:"sha256"`
}

type certificateTrustProjection struct {
	Kind string `json:"kind"`
	certificateObjectPin
	Bundle     []byte    `json:"bundle"`
	JournalUID types.UID `json:"journalUID,omitempty"`
	Revision   string    `json:"revision,omitempty"`
}

// certificateConsumerPin records Kubernetes observations, never a synthetic
// Certificate UID or a shared leaf digest for pod-local CSI certificates.
type certificateConsumerPin struct {
	WorkloadUID        types.UID `json:"workloadUID"`
	WorkloadGeneration int64     `json:"workloadGeneration"`
	PodName            string    `json:"podName"`
	PodUID             types.UID `json:"podUID"`
	ServiceAccount     string    `json:"serviceAccount"`
	ServiceAccountUID  types.UID `json:"serviceAccountUID"`
	Driver             string    `json:"driver,omitempty"`
	DriverUID          types.UID `json:"driverUID,omitempty"`
	ServerSPIFFEID     string    `json:"serverSpiffeID,omitempty"`
	TrustOnly          bool      `json:"trustOnly"`
}

func decodeCertificateLifecycle(journal *corev1.ConfigMap, namespace, release string, now time.Time) (certificateLifecycle, error) {
	var state certificateLifecycle
	if journal == nil || journal.UID == "" || journal.ResourceVersion == "" ||
		journal.Namespace != namespace || journal.Name != rotationJournalName ||
		journal.Labels[rotationOwner] != release || journal.DeletionTimestamp != nil ||
		len(journal.OwnerReferences) != 0 || len(journal.Data) != 1 || len(journal.BinaryData) != 0 ||
		journal.Annotations[helmReleaseNameAnnotation] != "" {
		return state, errors.New("CA lifecycle journal has foreign or incomplete ownership")
	}
	data := journal.Data[rotationJournalKey]
	if len(data) == 0 || len(data) > maxRotationBytes {
		return state, errors.New("CA lifecycle journal is empty or too large")
	}
	decoder := json.NewDecoder(strings.NewReader(data))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return state, errors.New("CA lifecycle journal must be an object")
	}
	if err := checkRotationJSON(decoder, first, 0); err != nil {
		return state, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return state, errors.New("CA lifecycle journal contains trailing data")
	}
	var version struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal([]byte(data), &version); err != nil || version.SchemaVersion != 4 {
		return state, errors.New("legacy or unsupported CA journal: restore the matching pre-token release for recovery; do not reinterpret certificate evidence as CSI evidence")
	}
	decoder = json.NewDecoder(strings.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return state, fmt.Errorf("invalid schema4 CA journal: %w", err)
	}
	if state.Namespace != namespace || state.Release != release {
		return state, errors.New("CA lifecycle journal belongs to another installation")
	}
	return state, state.validate(now)
}

func (state certificateLifecycle) validate(now time.Time) error {
	id, err := uuid.Parse(state.ID)
	if state.SchemaVersion != 4 || err != nil || id == uuid.Nil || id.String() != state.ID ||
		state.Namespace == "" || state.Release == "" || state.CreatedAt.IsZero() || state.CreatedAt.After(now) ||
		!(slices.Contains(rotationPhases, state.Phase) || state.Phase == certificatePrepare) ||
		!slices.Contains([]string{"radius", "external"}, state.Provider) ||
		!slices.Contains([]string{"selfSigned", "caSecret", "issuerRef"}, state.Mode) {
		return errors.New("invalid CA lifecycle identity, phase or provider")
	}
	names := []string{"aggregation"}
	if state.Provider == "radius" {
		names = append(names, "internal")
	}
	if len(state.Planes) != len(names) {
		return errors.New("CA lifecycle must pin every enabled trust plane")
	}
	if state.Execution != nil {
		if err := state.validateExecution(); err != nil {
			return err
		}
	} else if state.Phase == certificatePrepare {
		return errors.New("certificate preparation requires a durable execution intent")
	}
	affected := false
	for _, name := range names {
		plane, ok := state.Planes[name]
		if !ok {
			return fmt.Errorf("missing %s trust plane", name)
		}
		if plane.Affected {
			affected = true
		}
		if name == "aggregation" && plane.Affected && state.APIServiceUID == "" {
			return errors.New("aggregation change requires a pinned APIService")
		}
		if state.Phase == certificatePrepare {
			if err := state.validatePreparingPlane(name, plane, now); err != nil {
				return err
			}
			continue
		}
		phase := state.Phase
		// Retirement is a durable intent before either independent source is
		// changed. A plane retains valid overlap until its own CAS completes.
		if state.Execution != nil && phase == rotationRetire && plane.Affected &&
			plane.Source.Revision == state.ID+"/publish" {
			phase = rotationReady
		}
		if err := plane.validate(name, phase, now); err != nil {
			return err
		}
		if err := state.validateTargetSnapshot(name, plane); err != nil {
			return err
		}
		if state.Mode == "issuerRef" && plane.Signer != nil ||
			state.Mode != "issuerRef" && plane.Signer == nil {
			return errors.New("CA signer ownership differs from certificate mode")
		}
	}
	if !affected {
		return errors.New("CA lifecycle affects no trust plane")
	}
	seen := map[types.UID]bool{}
	for _, consumer := range state.Consumers {
		if state.Provider != "radius" || consumer.WorkloadUID == "" || consumer.WorkloadGeneration <= 0 ||
			consumer.ServiceAccountUID == "" || consumer.PodUID == "" ||
			seen[consumer.PodUID] || len(validation.IsDNS1123Subdomain(consumer.PodName)) != 0 ||
			len(validation.IsDNS1123Subdomain(consumer.ServiceAccount)) != 0 {
			return errors.New("invalid or duplicate CSI consumer identity")
		}
		seen[consumer.PodUID] = true
		if consumer.TrustOnly {
			if consumer.Driver != "" || consumer.DriverUID != "" || consumer.ServerSPIFFEID != "" {
				return errors.New("trust-only consumers must not claim CSI serving evidence")
			}
		} else if consumer.Driver == "" || consumer.DriverUID == "" || consumer.ServerSPIFFEID == "" {
			return errors.New("CSI receiver requires observed driver and server identity pins")
		}
	}
	return nil
}

func (plane certificateLifecyclePlane) validate(name string, phase rotationPhase, now time.Time) error {
	if plane.Issuer.Group != "cert-manager.io" ||
		!slices.Contains([]string{"Issuer", "ClusterIssuer"}, plane.Issuer.Kind) ||
		len(validation.IsDNS1123Subdomain(plane.Issuer.Name)) != 0 {
		return errors.New("invalid pinned target issuer")
	}
	if plane.Signer != nil && (plane.Signer.UID == "" || plane.Signer.ResourceVersion == "" ||
		plane.Signer.Name == "" || !validCertificateHash(plane.Signer.SHA256)) {
		return errors.New("local signer requires observed identity and public DER pins")
	}
	for _, bundle := range [][]byte{plane.OldCA, plane.NewCA} {
		certificates, err := parseCertificateBundlePEM(bundle)
		if err != nil || len(certificates) != 1 {
			return errors.New("each plane must pin exactly one previous and one target dedicated certificate")
		}
	}
	_, old, err := rotationHistoricalDigest(plane.OldCA)
	if err != nil || len(old) != 1 {
		return errors.New("each plane requires its own previous dedicated anchor")
	}
	_, next, err := mtls.TrustBundleDigest(plane.NewCA, now)
	if err != nil || len(next) != 1 || plane.Affected == slices.Equal(old, next) {
		return errors.New("affected plane must select a distinct currently valid CA; unaffected plane must preserve its CA")
	}
	if plane.Signer != nil && plane.Signer.SHA256 != next[0] {
		return errors.New("local signer public DER pin differs from the target CA")
	}
	want := plane.NewCA
	if plane.Affected && phase != rotationRetire && phase != rotationComplete {
		if _, _, err := mtls.TrustBundleDigest(plane.OldCA, now); err != nil {
			return errors.New("overlap cannot contain an expired old CA")
		}
		want = append(append([]byte{}, plane.OldCA...), plane.NewCA...)
	}
	if err := exactCertificateSet(plane.Source.Bundle, want, now); err != nil {
		return err
	}
	kind, source := "ConfigMap", "radius-mtls-ca"
	if name == "aggregation" {
		kind, source = "Secret", "radius-apiserver-trust"
	}
	if plane.Source.Kind != kind || plane.Source.Name != source || plane.Source.UID == "" ||
		plane.Source.ResourceVersion == "" || plane.Source.SHA256 != fmt.Sprintf("%x", sha256.Sum256(plane.Source.Bundle)) {
		return errors.New("public source requires observed identity and exact content pins")
	}
	return nil
}

func validCertificateHash(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}

// exactCertificateSet rejects duplicate, missing, additional and expired anchors;
// identical keys or permissive subset checks are not certificate identity.
func exactCertificateSet(actual, expected []byte, now time.Time) error {
	for _, bundle := range [][]byte{actual, expected} {
		certs, err := parseCertificateBundlePEM(bundle)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, cert := range certs {
			if seen[string(cert.Raw)] {
				return errors.New("duplicate CA anchor")
			}
			seen[string(cert.Raw)] = true
		}
	}
	a, _, err := mtls.TrustBundleDigest(actual, now)
	if err != nil {
		return err
	}
	b, _, err := mtls.TrustBundleDigest(expected, now)
	if err != nil || a != b {
		return errors.New("CA trust differs from the exact selected anchor set")
	}
	return nil
}

func certificateRotationPlanes(provider, scope string) ([]string, error) {
	if provider == "external" {
		if scope != "" && scope != "aggregation" {
			return nil, errors.New("external TLS has no Radius-managed internal CA")
		}
		return []string{"aggregation"}, nil
	}
	if provider != "radius" {
		return nil, errors.New("unsupported TLS provider")
	}
	switch scope {
	case "":
		return []string{"internal", "aggregation"}, nil
	case "internal", "aggregation":
		return []string{scope}, nil
	default:
		return nil, errors.New("unsupported CA rotation scope")
	}
}

func (i *Impl) runCertificateRotation(ctx context.Context, options ClusterOptions, kubeContext string, readOnly bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := options.Certificates.Validate(); err != nil {
		return err
	}
	flags := &genericclioptions.ConfigFlags{Namespace: &options.Radius.Namespace, Context: &kubeContext}
	configuration, err := initHelmConfig(flags)
	if err != nil {
		return err
	}
	release, err := i.Helm.RunHelmGet(configuration, radiusReleaseName)
	if err != nil {
		return err
	}
	if release == nil || release.Chart == nil {
		return errors.New("CA rotation requires an installed chart and its history")
	}
	if options.RadiusReleaseValues != nil && !reflect.DeepEqual(options.RadiusReleaseValues, release.Config) {
		return errors.New("installed certificate settings changed since preparation")
	}
	options.RadiusInstalled = true
	options.RadiusReleaseValues = copyValues(release.Config)
	values := copyValues(release.Config)
	target, imports, err := i.prepareCertificateTarget(ctx, options, release.Chart, values, kubeContext)
	if err != nil {
		return err
	}
	if options.Certificates.RotateCA && target.Mode != "selfSigned" {
		return errors.New("--rotate-ca only replaces Radius-owned selfSigned CAs; select a distinct staged BYO signer instead")
	}
	if _, err := certificateRotationPlanes(target.Provider, ""); err != nil {
		return err
	}
	store, err := i.rotationStoreForContext(kubeContext, options.Radius.Namespace)
	if err != nil {
		return err
	}
	journal, err := store.kube.CoreV1().ConfigMaps(store.namespace).Get(ctx, rotationJournalName, metav1.GetOptions{})
	var state certificateLifecycle
	if err == nil {
		if err := store.checkCertificateLifecycleRecovery(ctx, journal); err != nil {
			return err
		}
		state, err = decodeCertificateLifecycle(journal, store.namespace, store.release, store.now())
		if err != nil {
			return err
		}
	} else if apierrors.IsNotFound(err) {
		journal = nil
		if err := store.checkOrphanedCertificateSources(ctx); err != nil {
			return err
		}
	} else {
		return err
	}
	requested := rotationPhase(options.Certificates.RotationPhase)
	if requested == "" {
		requested = rotationPublish
	}
	if release.Chart.Metadata == nil || release.Chart.Metadata.Annotations["radius.io/ca-rotation-protocol"] != "4" {
		return errors.New("CA rotation requires an installed schema4-capable chart; first upgrade the chart without changing its CA selection")
	}
	if journal == nil || state.Phase == rotationComplete && state.Execution != nil &&
		state.Execution.FinalReleaseVersion != 0 && requested == rotationPublish {
		state, err = store.planCertificateLifecycle(ctx, release, release.Chart, target, "")
		if err != nil {
			return fmt.Errorf("cannot plan the selected certificate transition: %w", err)
		}
	} else if state.Execution == nil {
		return errors.New("the recorded lifecycle has no executable token/CSI resource intent")
	}
	if state.Mode != target.Mode || state.Provider != target.Provider {
		return errors.New("retry differs from the recorded certificate mode or provider")
	}
	targetHash, err := bootstrapDigest(target.Values)
	if err != nil || targetHash != state.Execution.TargetValuesSHA256 {
		return errors.New("retry changed the recorded certificate target values")
	}
	// No Lease, imported Secret, journal or source write is permitted before a
	// complete observation mechanism and authorized access have been supplied.
	// Schema2 UCP-server-trust evidence is not an all-authority implementation.
	writer := &certificateExecutionWriter{store: store, state: state, journal: journal}
	if readOnly {
		return errCertificateObserverUnavailable
	}
	executor := &certificateLifecycleExecutor{writer: writer, impl: i, options: options,
		selected: release.Chart, target: target, imports: imports, kubeContext: kubeContext}
	writer.observeServing = executor.observeServing
	return executor.run(ctx, requested)
}

func (s *rotationStore) checkLifecycleProjection(ctx context.Context, journal *corev1.ConfigMap, state certificateLifecycle, name string, plane certificateLifecyclePlane) error {
	var metadata metav1.Object
	var bundle []byte
	if name == "aggregation" {
		object, err := s.kube.CoreV1().Secrets(state.Namespace).Get(ctx, plane.Source.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if object.Type != corev1.SecretTypeOpaque || len(object.Data) != 1 || len(object.StringData) != 0 ||
			object.Annotations[allowDirectInjectionAnnotation] != "true" {
			return errors.New("aggregation trust source must contain only public ca.crt with direct-injection annotation")
		}
		metadata, bundle = object, object.Data[caCertificateKey]
	} else {
		object, err := s.kube.CoreV1().ConfigMaps(state.Namespace).Get(ctx, plane.Source.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if len(object.Data) != 1 || len(object.BinaryData) != 0 {
			return errors.New("internal trust source must contain only public ca.crt")
		}
		metadata, bundle = object, []byte(object.Data[caCertificateKey])
	}
	annotations := metadata.GetAnnotations()
	if metadata.GetDeletionTimestamp() != nil || len(metadata.GetOwnerReferences()) != 0 ||
		metadata.GetLabels()[helmManagedByLabel] != helmManagedByValue ||
		annotations[helmReleaseNameAnnotation] != state.Release ||
		annotations[helmReleaseNamespaceAnnotation] != state.Namespace ||
		annotations["radius.io/certificate-mode"] != state.Mode ||
		annotations[rotationJournalUID] != string(journal.UID) ||
		annotations[rotationRevision] != state.ID+"/"+string(state.Phase) ||
		metadata.GetResourceVersion() == "" || plane.Source.UID != metadata.GetUID() ||
		!bytes.Equal(plane.Source.Bundle, bundle) {
		return errors.New("public trust source differs from the owned journal snapshot")
	}
	// Completed snapshots survive unrelated metadata updates. Active transitions
	// still require their exact observed revision before any conditional write.
	if state.Phase != rotationComplete && plane.Source.ResourceVersion != metadata.GetResourceVersion() {
		return errors.New("public trust source revision changed during the active lifecycle")
	}
	return nil
}
