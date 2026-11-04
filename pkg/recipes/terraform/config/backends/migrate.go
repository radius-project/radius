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

package backends

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
	"github.com/radius-project/radius/pkg/version"
)

// Labels, annotations, and data keys of the Terraform Kubernetes backend. See
// https://github.com/hashicorp/terraform/blob/main/internal/backend/remote-state/kubernetes/client.go.
const (
	tfstateLabel              = "tfstate"
	tfstateSecretSuffixLabel  = "tfstateSecretSuffix"
	tfstateWorkspaceLabel     = "tfstateWorkspace"
	tfstateManagedByLabel     = "app.kubernetes.io/managed-by"
	tfstateLockInfoAnnotation = "app.terraform.io/lock-info"
	tfstateDataKey            = "tfstate"
	tfstateWorkspace          = "default"
	tfstateLockNamePrefix     = "lock-"
	tfstateChunkSeparator     = "-part-"
)

const (
	// StateMigrationHolderPrefix starts the holder identity of every Terraform state lock that the SHA-256 migration
	// holds.
	StateMigrationHolderPrefix = "radius-sha256-migration/"

	// StateMigrationTombstone is the holder identity of the lock that the migration keeps on the legacy name of a state
	// after moving the state. Terraform cannot acquire the lock, so a deployment that resolved the legacy name before
	// the move fails with a lock error that names the new Secret, instead of running against an empty state.
	StateMigrationTombstone = StateMigrationHolderPrefix + "moved"

	// AnnotationStateMigratedFrom records, on each moved state Secret, the name of the Secret it was copied from.
	AnnotationStateMigratedFrom = "radapp.io/sha256-migrated-from"

	// AnnotationStateMigratedFromUID records, on each moved state Secret, the UID of the Secret it was copied from.
	AnnotationStateMigratedFromUID = "radapp.io/sha256-migrated-from-uid"

	// AnnotationStateMigratedFromVersion records, on each moved state Secret, the resource version of the Secret it was
	// copied from.
	AnnotationStateMigratedFromVersion = "radapp.io/sha256-migrated-from-version"

	// LabelStateMigration marks the lock that the migration keeps on the legacy name of a moved state.
	LabelStateMigration = "radapp.io/sha256-migration"

	// stateMigrationLeaseDuration is how long another migration runner waits before taking over the lock of a runner
	// that stopped while moving a state. Terraform ignores the duration.
	stateMigrationLeaseDuration = 5 * time.Minute
)

var (
	errStateLocked     = errors.New("the Terraform state is locked")
	errUnexpectedState = errors.New("unexpected Terraform state Secrets")
)

// StateMigrationStepName is the name of the migration step that moves Terraform states.
const StateMigrationStepName = "terraformState"

// NewStateMigrationStep returns the migration step that moves the Terraform states of the resources a component
// stores. client returns a client for the cluster that holds the states, and db returns the component's database.
func NewStateMigrationStep(client func() (kubernetes.Interface, error), db func(context.Context) (database.Client, error)) hashmigration.Step {
	var migrator *StateMigrator
	return hashmigration.StepFunc{
		StepName: StateMigrationStepName,
		Fn: func(ctx context.Context) (hashmigration.Result, error) {
			databaseClient, err := db(ctx)
			if err != nil {
				return hashmigration.Result{}, err
			}

			if _, ok := databaseClient.(database.Enumerator); !ok {
				return hashmigration.Result{NotApplicable: true, Message: "the database provider cannot enumerate resources"}, nil
			}

			// The migrator keeps its lock holder identity across passes.
			if migrator == nil {
				kubernetesClient, err := client()
				if err != nil {
					return hashmigration.Result{}, err
				}

				migrator = NewStateMigrator(kubernetesClient)
			}

			return migrator.MigrateLegacyStates(ctx, databaseClient)
		},
	}
}

// StateMigrator moves Terraform states that the Kubernetes backend stores under legacy (SHA-1) secret suffixes to
// their current (SHA-256) suffixes.
//
// A move holds the Terraform lock of the legacy state, copies the state Secrets to their current names, deletes the
// legacy Secrets, and keeps the legacy lock as a tombstone. It is idempotent, resumes an interrupted move, and is safe
// to run concurrently with deployments and with other migrators. See https://github.com/radius-project/radius/issues/8084.
type StateMigrator struct {
	client    kubernetes.Interface
	namespace string
	holder    string
	now       func() time.Time
}

// NewStateMigrator creates a StateMigrator for the states in the Radius namespace.
func NewStateMigrator(client kubernetes.Interface) *StateMigrator {
	return &StateMigrator{
		client:    client,
		namespace: RadiusNamespace,
		holder:    StateMigrationHolderPrefix + uuid.New().String(),
		now:       time.Now,
	}
}

// MigrateLegacyStates moves the Terraform state of every resource in db whose state is stored under its legacy secret
// suffix. The suffix is computed from the resource ID and the environment and application in the resource properties,
// as it is for a deployment. The state of a resource with an operation in progress is moved on a later pass, because
// the operation may already have resolved the legacy name. db must implement database.Enumerator.
func (m *StateMigrator) MigrateLegacyStates(ctx context.Context, db database.Client) (hashmigration.Result, error) {
	enumerator, ok := db.(database.Enumerator)
	if !ok {
		return hashmigration.Result{}, errors.New("the database provider cannot enumerate resources")
	}

	stored, err := m.storedSuffixes(ctx)
	if err != nil {
		return hashmigration.Result{}, err
	}

	if len(stored) == 0 {
		return hashmigration.Result{}, nil
	}

	moves := map[string]stateMove{}
	err = enumerator.Enumerate(ctx, func(obj *database.Object) error {
		owner, ok := stateOwnerOf(obj)
		if !ok {
			return nil
		}

		current, legacy, err := SecretSuffixes(owner.metadata)
		if err != nil {
			// Not a resource that can have a Terraform state.
			return nil
		}

		if stored[legacy] {
			// A resource stored under both its legacy and current database names is visited twice.
			move := moves[legacy]
			move.current = current
			move.owner = obj.ID
			move.busy = move.busy || owner.busy
			moves[legacy] = move
		}

		return nil
	})
	if err != nil {
		return hashmigration.Result{}, err
	}

	result := hashmigration.Result{}
	for _, legacy := range slices.Sorted(maps.Keys(moves)) {
		move := moves[legacy]
		if !move.busy {
			// An operation that started after the resource was enumerated may resolve the legacy name before the
			// move. Read the resource again right before the move to keep that window small.
			move.busy, err = m.ownerBusy(ctx, db, move.owner)
			if errors.Is(err, &database.ErrNotFound{}) {
				// The resource was deleted, and its deletion removes the state.
				continue
			} else if err != nil {
				result.Remaining++
				result.Message = err.Error()
				continue
			}
		}

		if move.busy {
			result.Remaining++
			result.Message = fmt.Sprintf("an operation is in progress on the resource that owns the Terraform state in %s", stateSecretName(legacy, 0))
			continue
		}

		moved := m.MoveState(ctx, legacy, move.current)
		result.Add(moved)
		if moved.Message != "" {
			result.Message = moved.Message
		}
	}

	return result, nil
}

// ownerBusy reports whether an operation is in progress on the resource with the given ID.
func (m *StateMigrator) ownerBusy(ctx context.Context, db database.Client, id string) (bool, error) {
	obj, err := db.Get(ctx, id)
	if err != nil {
		return false, err
	}

	owner, ok := stateOwnerOf(obj)
	return ok && owner.busy, nil
}

// stateMove is a Terraform state to move to its current secret suffix.
type stateMove struct {
	current string

	// owner is the ID the resource that owns the state is stored under.
	owner string

	// busy records that an operation is in progress on the resource that owns the state.
	busy bool
}

// MoveState moves the state stored under legacySuffix to currentSuffix. A state that cannot be moved now, for example
// because a deployment holds its lock, is counted as remaining and moved on a later pass.
func (m *StateMigrator) MoveState(ctx context.Context, legacySuffix string, currentSuffix string) hashmigration.Result {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("legacySecret", stateSecretName(legacySuffix, 0), "currentSecret", stateSecretName(currentSuffix, 0))

	lock, err := m.lock(ctx, legacySuffix, currentSuffix)
	if err != nil {
		logger.Info("Unable to lock the Terraform state; the move will be retried", "reason", err.Error())
		return hashmigration.Result{Remaining: 1, Message: err.Error()}
	}

	result, moved, err := m.moveLocked(ctx, legacySuffix, currentSuffix)
	if errors.Is(err, errUnexpectedState) {
		logger.Info("The Terraform state cannot be moved automatically", "reason", err.Error())
		result = hashmigration.Result{Skipped: 1, Message: err.Error()}
	} else if err != nil {
		logger.Error(err, "Unable to move the Terraform state; the move will be retried")
		result = hashmigration.Result{Remaining: 1, Message: err.Error()}
	} else if result.Conflicts > 0 {
		logger.Info("The Terraform state is stored under both names with different content; Radius uses the current name and leaves the legacy state in place")
	} else if result.Migrated > 0 || result.Deduplicated > 0 {
		logger.Info("Moved the Terraform state")
	}

	if err := m.unlock(ctx, lock, legacySuffix, currentSuffix, moved); err != nil {
		// The lock stays held by this migrator. Terraform cannot acquire it, and another migrator takes it over once
		// it expires, so the state remains protected.
		logger.Error(err, "Unable to release the Terraform state lock")
	}

	return result
}

// moveLocked moves the state while the migrator holds the lock of the legacy state. It reports whether the state is
// stored under its current name afterwards, in which case the legacy lock becomes a tombstone.
func (m *StateMigrator) moveLocked(ctx context.Context, legacySuffix string, currentSuffix string) (hashmigration.Result, bool, error) {
	legacy, err := m.listChunks(ctx, legacySuffix)
	if err != nil {
		return hashmigration.Result{}, false, err
	}

	current, err := m.listChunks(ctx, currentSuffix)
	if err != nil {
		return hashmigration.Result{}, false, err
	}

	if len(legacy) == 0 {
		// Moved or deleted since the Secrets were listed.
		_, hasCurrent := current[0]
		return hashmigration.Result{}, hasCurrent, nil
	}

	legacyChunks, ok := orderedChunks(legacy)
	if !ok {
		return hashmigration.Result{}, false, fmt.Errorf("%w: the state stored under %s has missing parts", errUnexpectedState, stateSecretName(legacySuffix, 0))
	}

	if first, hasCurrent := current[0]; hasCurrent {
		result := hashmigration.Result{}
		currentChunks, ok := orderedChunks(current)
		switch {
		case copiedFrom(first, legacyChunks[0]):
			// An earlier move copied this legacy state and stopped before deleting all of it. The migration's lock has
			// protected the legacy state since, while deployments may have changed the copy, so only the legacy state is
			// left to delete.
			result.Migrated = 1
		case ok && sameState(currentChunks, legacyChunks):
			// The same state is stored under both names.
			result.Deduplicated = 1
		default:
			return hashmigration.Result{Conflicts: 1}, false, nil
		}

		if err := m.deleteChunks(ctx, legacyChunks); err != nil {
			return hashmigration.Result{}, true, err
		}

		return result, true, nil
	}

	if err := m.copyChunks(ctx, legacyChunks, current, currentSuffix); err != nil {
		return hashmigration.Result{}, false, err
	}

	if err := m.deleteChunks(ctx, legacyChunks); err != nil {
		// The copy is complete and Radius uses it. The next pass deletes the legacy state.
		return hashmigration.Result{}, true, err
	}

	return hashmigration.Result{Migrated: 1}, true, nil
}

// copyChunks copies the legacy state to the current name. partial holds copies left by an interrupted move.
func (m *StateMigrator) copyChunks(ctx context.Context, legacy []corev1.Secret, partial map[int]corev1.Secret, currentSuffix string) error {
	secrets := m.client.CoreV1().Secrets(m.namespace)
	for index, existing := range partial {
		if index >= len(legacy) || existing.Annotations[AnnotationStateMigratedFromUID] != string(legacy[index].UID) {
			return fmt.Errorf("%w: %s exists but was not copied from %s", errUnexpectedState, existing.Name, legacy[min(index, len(legacy)-1)].Name)
		}
	}

	written := []string{}
	rollback := func() {
		for _, name := range written {
			_ = secrets.Delete(ctx, name, metav1.DeleteOptions{})
		}
	}

	// Radius uses the current name as soon as its first Secret exists, so that Secret is created last, once every other
	// part of the state is in place.
	for index := len(legacy) - 1; index >= 0; index-- {
		copied := stateCopy(legacy[index], currentSuffix, index)
		var err error
		if existing, ok := partial[index]; ok {
			copied.ResourceVersion = existing.ResourceVersion
			_, err = secrets.Update(ctx, copied, metav1.UpdateOptions{})
		} else {
			_, err = secrets.Create(ctx, copied, metav1.CreateOptions{})
		}

		if err != nil {
			rollback()
			return fmt.Errorf("failed to copy %s to %s: %w", legacy[index].Name, copied.Name, err)
		}

		written = append(written, copied.Name)
	}

	return nil
}

// deleteChunks deletes a legacy state. The first Secret is deleted last, so an interrupted delete leaves a state that
// a later pass recognizes and finishes deleting.
func (m *StateMigrator) deleteChunks(ctx context.Context, chunks []corev1.Secret) error {
	secrets := m.client.CoreV1().Secrets(m.namespace)
	for index := len(chunks) - 1; index >= 0; index-- {
		chunk := chunks[index]
		err := secrets.Delete(ctx, chunk.Name, metav1.DeleteOptions{
			Preconditions: &metav1.Preconditions{UID: &chunk.UID, ResourceVersion: &chunk.ResourceVersion},
		})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete %s: %w", chunk.Name, err)
		}
	}

	return nil
}

// stateLock is a Terraform state lock held by the migrator.
type stateLock struct {
	lease *coordinationv1.Lease

	// wasTombstone records that the lock was a tombstone before the migrator acquired it.
	wasTombstone bool
}

// lock acquires the Terraform lock of the legacy state the way Terraform does, so deployments and the migrator
// exclude each other.
func (m *StateMigrator) lock(ctx context.Context, legacySuffix string, currentSuffix string) (*stateLock, error) {
	leases := m.client.CoordinationV1().Leases(m.namespace)
	name := stateLockName(legacySuffix)
	info := fmt.Sprintf("Radius is moving this Terraform state to Kubernetes Secret %s to migrate state names from SHA-1 to SHA-256.", stateSecretName(currentSuffix, 0))

	lease, err := leases.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		lease = &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: m.namespace, Labels: stateLabels(legacySuffix)}}
		if err := m.hold(lease, m.holder, info); err != nil {
			return nil, err
		}

		created, err := leases.Create(ctx, lease, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			return nil, errStateLocked
		} else if err != nil {
			return nil, err
		}

		return &stateLock{lease: created}, nil
	} else if err != nil {
		return nil, err
	}

	holder := ""
	if lease.Spec.HolderIdentity != nil {
		holder = *lease.Spec.HolderIdentity
	}

	wasTombstone := holder == StateMigrationTombstone
	switch {
	case holder == "", wasTombstone, holder == m.holder:
	case strings.HasPrefix(holder, StateMigrationHolderPrefix) && m.expired(lease):
		// A migrator stopped while moving the state. moveLocked finishes or restarts its move.
	default:
		return nil, fmt.Errorf("%w by %s", errStateLocked, holder)
	}

	if err := m.hold(lease, m.holder, info); err != nil {
		return nil, err
	}

	updated, err := leases.Update(ctx, lease, metav1.UpdateOptions{})
	if apierrors.IsConflict(err) {
		return nil, errStateLocked
	} else if err != nil {
		return nil, err
	}

	return &stateLock{lease: updated, wasTombstone: wasTombstone}, nil
}

// unlock releases the lock. When the state is stored under its current name, or the lock was a tombstone, the lock
// becomes a tombstone. Otherwise it is released the way Terraform releases it.
func (m *StateMigrator) unlock(ctx context.Context, lock *stateLock, legacySuffix string, currentSuffix string, moved bool) error {
	lease := lock.lease.DeepCopy()
	if moved || lock.wasTombstone {
		info := fmt.Sprintf("Radius moved this Terraform state to Kubernetes Secret %s when it migrated state names from SHA-1 to SHA-256. Retry the operation to use the moved state.", stateSecretName(currentSuffix, 0))
		if err := m.hold(lease, StateMigrationTombstone, info); err != nil {
			return err
		}

		if lease.Labels == nil {
			lease.Labels = stateLabels(legacySuffix)
		}
		lease.Labels[LabelStateMigration] = "tombstone"
	} else {
		lease.Spec.HolderIdentity = nil
		lease.Spec.AcquireTime = nil
		lease.Spec.RenewTime = nil
		lease.Spec.LeaseDurationSeconds = nil
		delete(lease.Annotations, tfstateLockInfoAnnotation)
	}

	_, err := m.client.CoordinationV1().Leases(m.namespace).Update(ctx, lease, metav1.UpdateOptions{})
	return err
}

// terraformLockInfo has the fields of the lock information Terraform records on a lock and prints in a lock error.
type terraformLockInfo struct {
	ID        string
	Operation string
	Info      string
	Who       string
	Version   string
	Created   time.Time
	Path      string
}

// hold makes holder the holder of lease and records lock information Terraform can print.
func (m *StateMigrator) hold(lease *coordinationv1.Lease, holder string, info string) error {
	now := metav1.NewMicroTime(m.now())
	duration := int32(stateMigrationLeaseDuration.Seconds())
	lease.Spec.HolderIdentity = &holder
	lease.Spec.AcquireTime = &now
	lease.Spec.RenewTime = &now
	lease.Spec.LeaseDurationSeconds = &duration

	b, err := json.Marshal(terraformLockInfo{
		ID:        holder,
		Operation: "RadiusSHA256Migration",
		Info:      info,
		Who:       "radius",
		Version:   version.Version(),
		Created:   now.UTC(),
	})
	if err != nil {
		return err
	}

	if lease.Annotations == nil {
		lease.Annotations = map[string]string{}
	}
	lease.Annotations[tfstateLockInfoAnnotation] = string(b)
	return nil
}

func (m *StateMigrator) expired(lease *coordinationv1.Lease) bool {
	renewed := lease.Spec.RenewTime
	if renewed == nil {
		renewed = lease.Spec.AcquireTime
	}

	if renewed == nil || lease.Spec.LeaseDurationSeconds == nil {
		return true
	}

	return m.now().After(renewed.Add(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second))
}

// storedSuffixes returns the secret suffixes of the states stored in the Radius namespace.
func (m *StateMigrator) storedSuffixes(ctx context.Context) (map[string]bool, error) {
	selector := labels.Set{tfstateLabel: "true", tfstateWorkspaceLabel: tfstateWorkspace}.String()
	suffixes := map[string]bool{}
	options := metav1.ListOptions{LabelSelector: selector, Limit: 250}
	for {
		list, err := m.client.CoreV1().Secrets(m.namespace).List(ctx, options)
		if err != nil {
			return nil, fmt.Errorf("failed to list Terraform state Secrets: %w", err)
		}

		for _, secret := range list.Items {
			if suffix := secret.Labels[tfstateSecretSuffixLabel]; suffix != "" {
				suffixes[suffix] = true
			}
		}

		if list.Continue == "" {
			return suffixes, nil
		}

		options.Continue = list.Continue
	}
}

// listChunks returns the Secrets that hold the state stored under suffix, by part index.
func (m *StateMigrator) listChunks(ctx context.Context, suffix string) (map[int]corev1.Secret, error) {
	list, err := m.client.CoreV1().Secrets(m.namespace).List(ctx, metav1.ListOptions{LabelSelector: stateSelector(suffix)})
	if err != nil {
		return nil, fmt.Errorf("failed to list the Secrets of Terraform state %s: %w", stateSecretName(suffix, 0), err)
	}

	chunks := map[int]corev1.Secret{}
	for _, secret := range list.Items {
		index, ok := stateChunkIndex(secret.Name, suffix)
		if !ok {
			return nil, fmt.Errorf("%w: %s is labeled as part of the state stored under %s", errUnexpectedState, secret.Name, stateSecretName(suffix, 0))
		}

		chunks[index] = secret
	}

	return chunks, nil
}

// orderedChunks returns the parts of a state in order, and whether the state has every part.
func orderedChunks(chunks map[int]corev1.Secret) ([]corev1.Secret, bool) {
	ordered := make([]corev1.Secret, len(chunks))
	for index, chunk := range chunks {
		if index >= len(ordered) {
			return nil, false
		}

		ordered[index] = chunk
	}

	return ordered, true
}

func sameState(a []corev1.Secret, b []corev1.Secret) bool {
	return slices.EqualFunc(a, b, func(x corev1.Secret, y corev1.Secret) bool {
		return bytes.Equal(x.Data[tfstateDataKey], y.Data[tfstateDataKey])
	})
}

// copiedFrom reports whether the migration made copy from exactly this version of source.
func copiedFrom(copy corev1.Secret, source corev1.Secret) bool {
	return copy.Annotations[AnnotationStateMigratedFromUID] == string(source.UID) &&
		copy.Annotations[AnnotationStateMigratedFromVersion] == source.ResourceVersion
}

// stateCopy returns a copy of a part of a state, named and labeled for currentSuffix.
func stateCopy(source corev1.Secret, currentSuffix string, index int) *corev1.Secret {
	copied := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        stateSecretName(currentSuffix, index),
			Namespace:   source.Namespace,
			Labels:      maps.Clone(source.Labels),
			Annotations: maps.Clone(source.Annotations),
		},
		Type: source.Type,
		Data: maps.Clone(source.Data),
	}

	if copied.Labels == nil {
		copied.Labels = map[string]string{}
	}
	copied.Labels[tfstateSecretSuffixLabel] = currentSuffix

	if copied.Annotations == nil {
		copied.Annotations = map[string]string{}
	}
	copied.Annotations[AnnotationStateMigratedFrom] = source.Name
	copied.Annotations[AnnotationStateMigratedFromUID] = string(source.UID)
	copied.Annotations[AnnotationStateMigratedFromVersion] = source.ResourceVersion

	return copied
}

// stateSecretName returns the name of a part of the state stored under suffix, as the Terraform Kubernetes backend
// names it.
func stateSecretName(suffix string, index int) string {
	name := KubernetesBackendNamePrefix + suffix
	if index > 0 {
		name += tfstateChunkSeparator + strconv.Itoa(index)
	}

	return name
}

func stateChunkIndex(name string, suffix string) (int, bool) {
	base := stateSecretName(suffix, 0)
	if name == base {
		return 0, true
	}

	part, ok := strings.CutPrefix(name, base+tfstateChunkSeparator)
	if !ok {
		return 0, false
	}

	index, err := strconv.Atoi(part)
	if err != nil || index <= 0 || strconv.Itoa(index) != part {
		return 0, false
	}

	return index, true
}

func stateLockName(suffix string) string {
	return tfstateLockNamePrefix + stateSecretName(suffix, 0)
}

func stateLabels(suffix string) map[string]string {
	return map[string]string{
		tfstateLabel:             "true",
		tfstateSecretSuffixLabel: suffix,
		tfstateWorkspaceLabel:    tfstateWorkspace,
		tfstateManagedByLabel:    "terraform",
	}
}

func stateSelector(suffix string) string {
	return labels.Set{tfstateLabel: "true", tfstateSecretSuffixLabel: suffix, tfstateWorkspaceLabel: tfstateWorkspace}.String()
}

// stateOwner is a resource that can own a Terraform state.
type stateOwner struct {
	// metadata holds the inputs of the secret suffix of the state.
	metadata *recipes.ResourceMetadata

	// busy records that an operation is in progress on the resource.
	busy bool
}

// stateOwnerOf returns the resource stored in obj, if it belongs to an environment and so can own a Terraform state.
func stateOwnerOf(obj *database.Object) (stateOwner, bool) {
	b, err := json.Marshal(obj.Data)
	if err != nil {
		return stateOwner{}, false
	}

	resource := struct {
		ID                string `json:"id"`
		ProvisioningState any    `json:"provisioningState"`
		Properties        struct {
			Environment any `json:"environment"`
			Application any `json:"application"`
		} `json:"properties"`
	}{}
	if err := json.Unmarshal(b, &resource); err != nil {
		return stateOwner{}, false
	}

	environment, _ := resource.Properties.Environment.(string)
	application, _ := resource.Properties.Application.(string)
	state, _ := resource.ProvisioningState.(string)

	// A deployment computes the suffix from the ID in the resource data, which is also the ID it is stored under.
	id := resource.ID
	if id == "" {
		id = obj.ID
	}

	if environment == "" || id == "" {
		return stateOwner{}, false
	}

	return stateOwner{
		metadata: &recipes.ResourceMetadata{ResourceID: id, EnvironmentID: environment, ApplicationID: application},
		busy:     !v1.ProvisioningState(state).IsTerminal(),
	}, true
}
