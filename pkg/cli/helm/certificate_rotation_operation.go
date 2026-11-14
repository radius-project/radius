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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

const (
	identityOperationLeaseName = "radius-identity-operation"
	identityProviderJournal    = "radius-provider-transition"
	identityProviderMarker     = "radius.io/provider-transition"
	identityProviderStateKey   = "state.json"
	maxIdentityProviderBytes   = 64 * 1024
)

type identityOperation struct {
	kube  kubernetes.Interface
	lease *coordinationv1.Lease
}

// withIdentityOperation shares the A10/A11 exclusion primitive, not their state
// machines. Existing leases are never renewed, adopted or stolen after expiry.
func withIdentityOperation(ctx context.Context, kube kubernetes.Interface, namespace string, run func(*identityOperation) error) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if run == nil {
		return errors.New("identity operation requires a guarded callback")
	}
	client := kube.CoordinationV1().Leases(namespace)
	holder := uuid.NewString()
	lease, err := client.Create(ctx, &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: identityOperationLeaseName, Namespace: namespace},
		Spec:       coordinationv1.LeaseSpec{HolderIdentity: &holder},
	}, metav1.CreateOptions{FieldManager: FieldManager})
	if err != nil {
		return fmt.Errorf("cannot acquire identity operation lock; do not steal an existing lease: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		deleteErr := client.Delete(cleanup, lease.Name, metav1.DeleteOptions{
			Preconditions: &metav1.Preconditions{UID: &lease.UID, ResourceVersion: &lease.ResourceVersion},
		})
		if deleteErr != nil {
			err = errors.Join(err, fmt.Errorf("cannot release identity operation lock: %w", deleteErr))
		}
	}()
	operation := &identityOperation{kube: kube, lease: lease}
	if err := operation.check(ctx); err != nil {
		return err
	}
	return run(operation)
}

func (o *identityOperation) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if o == nil || o.lease == nil || o.lease.Name != identityOperationLeaseName ||
		o.lease.UID == "" || o.lease.ResourceVersion == "" || ptr.Deref(o.lease.Spec.HolderIdentity, "") == "" {
		return errors.New("identity operation has no pinned lock")
	}
	live, err := o.kube.CoordinationV1().Leases(o.lease.Namespace).Get(ctx, o.lease.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if live.UID != o.lease.UID || live.ResourceVersion != o.lease.ResourceVersion ||
		live.DeletionTimestamp != nil || ptr.Deref(live.Spec.HolderIdentity, "") != *o.lease.Spec.HolderIdentity {
		return errors.New("identity operation lock changed; stop and investigate before retrying")
	}
	return ctx.Err()
}

func (s *rotationStore) checkWrite(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.guardWrite == nil {
		return errors.New("CA writes require the shared identity operation lock and provider journal guard")
	}
	if err := s.guardWrite(ctx); err != nil {
		return err
	}
	_, initial, err := readInitialCertificateJournal(ctx, s)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err == nil && initial.Phase != "complete" {
		return errors.New("finish the initial certificate installation before changing CA trust")
	}
	bootstrap, found, err := s.bootstrapIntent(ctx)
	if err != nil {
		return err
	}
	if found && bootstrap.Phase != bootstrapComplete && bootstrap.ID != s.activeBootstrap {
		return errors.New("finish the aggregation bootstrap before changing CA trust")
	}
	return nil
}

// checkProviderJournal is the CA-side inter-phase guard. The callback must run
// A11's full provider schema/identity validator on this exact observed document.
// Until integrated, a missing callback blocks every existing provider journal.
// Invoke this while holding the shared lease, before each CA phase/write.
func (o *identityOperation) checkProviderJournal(ctx context.Context, release string, validate func(*corev1.ConfigMap) error) error {
	if err := o.check(ctx); err != nil {
		return err
	}
	cm, err := o.kube.CoreV1().ConfigMaps(o.lease.Namespace).Get(ctx, identityProviderJournal, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return o.check(ctx)
	}

	if err != nil {
		return fmt.Errorf("cannot inspect provider transition journal: %w", err)
	}
	raw := cm.Data[identityProviderStateKey]
	if cm.UID == "" || cm.ResourceVersion == "" || cm.DeletionTimestamp != nil ||
		cm.Labels[identityProviderMarker] != release || len(cm.OwnerReferences) != 0 ||
		cm.Annotations["meta.helm.sh/release-name"] != "" || len(cm.Data) != 1 ||
		len(cm.BinaryData) != 0 || len(raw) == 0 || len(raw) > maxIdentityProviderBytes {
		return errors.New("invalid provider transition journal ownership or content")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return errors.New("provider transition journal must contain an object")
	}
	if err := checkRotationJSON(decoder, first, 0); err != nil {
		return fmt.Errorf("invalid provider transition JSON: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing provider transition journal content")
	}
	// Plan semantics remain owned by A11. This envelope only prevents an
	// accidentally permissive callback from authorizing an unfinished phase.
	var envelope struct {
		Version int             `json:"version"`
		Plan    json.RawMessage `json:"plan"`
		Phase   string          `json:"phase"`
	}
	decoder = json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return fmt.Errorf("invalid provider transition envelope: %w", err)
	}
	if envelope.Version != 2 {
		return errors.New("legacy or unsupported provider journal; preserve v1 state and use the matching pre-token release for investigation")
	}
	if envelope.Phase != "complete" || len(envelope.Plan) == 0 {
		return errors.New("finish the provider transition before changing CA trust")
	}
	if validate == nil {
		return errors.New("provider transition requires its integrated full journal validator before CA changes")
	}
	if err := validate(cm.DeepCopy()); err != nil {
		return fmt.Errorf("invalid completed provider transition: %w", err)
	}
	current, err := o.kube.CoreV1().ConfigMaps(o.lease.Namespace).Get(ctx, cm.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if current.UID != cm.UID || current.ResourceVersion != cm.ResourceVersion {
		return errors.New("provider transition journal changed during CA operation checks")
	}
	return o.check(ctx)
}
