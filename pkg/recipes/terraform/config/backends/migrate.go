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
	"sync"
	"time"
	"uuid"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
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

	// AnnotationStateMigratedParts records the UID and resource version of every legacy Secret a move copied, in part
	// order. The first Secret of a copy carries it from the moment the copy is published, and the tombstone lock
	// carries it once the move is committed.
	AnnotationStateMigratedParts = "radapp.io/sha256-migrated-parts"

	// LabelStateMigration marks the lock that the migration keeps on the legacy name of a moved state.
	LabelStateMigration = "radapp.io/sha256-migration"

	// stateMigrationLeaseDuration is how long another migration runner waits before taking over the lock of a runner
	// that stopped while moving a state. Terraform ignores the duration.
	stateMigrationLeaseDuration = 5 * time.Minute

	// stateMigrationCallTimeout bounds every Kubernetes call of a move. A move renews its lock before each write, so a
	// stalled call ends long before the lock can expire and be taken over.
	stateMigrationCallTimeout = 30 * time.Second
)

var (
	errStateLocked     = errors.New("the Terraform state is locked")
	errUnexpectedState = errors.New("unexpected Terraform state Secrets")
	errLockLost        = errors.New("another migrator took over the Terraform state lock")
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
// A move holds the Terraform lock of the legacy state and renews it before every write. It copies the state Secrets to
// their current names, creating the first Secret last so that a deployment only ever finds a complete copy, records
// the commit of the move on the lock, deletes the legacy Secrets, and keeps the lock as a tombstone. A move is
// idempotent: a later pass finishes or removes an interrupted copy, and a copy whose creation had an uncertain outcome
// keeps the legacy state locked until a later pass resolves it. It is safe to run concurrently with deployments and
// with other migrators. See https://github.com/radius-project/radius/issues/8084.
type StateMigrator struct {
	client    kubernetes.Interface
	namespace string
	holder    string
	now       func() time.Time

	mu sync.Mutex

	// unsettled records, by lock name, until when the first Secret of a copy that this migrator created with an
	// uncertain outcome may still be applied. Every write follows a renewal of the lock and is applied, if at all,
	// within stateMigrationLeaseDuration, which is also how long another migrator waits before it takes over a lock.
	unsettled map[string]time.Time

	// kept records, by legacy suffix, the current suffix of each state whose lock this migrator kept after a pass. A
	// later pass resolves the move even if the resource that owns the state was deleted meanwhile, because Terraform
	// never expires the lock.
	kept map[string]string
}

// NewStateMigrator creates a StateMigrator for the states in the Radius namespace.
func NewStateMigrator(client kubernetes.Interface) *StateMigrator {
	return &StateMigrator{
		client:    client,
		namespace: RadiusNamespace,
		holder:    StateMigrationHolderPrefix + uuid.New().String(),
		now:       time.Now,
		unsettled: map[string]time.Time{},
		kept:      map[string]string{},
	}
}

// keep records that this migrator keeps the lock of the state stored under legacySuffix.
func (m *StateMigrator) keep(legacySuffix string, currentSuffix string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.kept[legacySuffix] = currentSuffix
}

// forget records that this migrator no longer keeps the lock of the state stored under legacySuffix.
func (m *StateMigrator) forget(legacySuffix string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.kept, legacySuffix)
}

// keptMoves returns the states whose lock this migrator keeps, by legacy suffix.
func (m *StateMigrator) keptMoves() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return maps.Clone(m.kept)
}

// markUnsettled records that the first Secret of a copy may still be created under the lock name.
func (m *StateMigrator) markUnsettled(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unsettled[name] = m.now().Add(stateMigrationLeaseDuration)
}

// isUnsettled reports whether the first Secret of a copy that this migrator created under the lock name with an
// uncertain outcome may still be applied.
func (m *StateMigrator) isUnsettled(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	until, ok := m.unsettled[name]
	if ok && !m.now().Before(until) {
		delete(m.unsettled, name)
		return false
	}
	return ok
}

// clearUnsettled records that the move under the lock name was committed.
func (m *StateMigrator) clearUnsettled(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.unsettled, name)
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

	moves := map[string]stateMove{}
	enumerate := enumerator.Enumerate
	if len(stored) == 0 {
		enumerate = func(context.Context, func(*database.Object) error) error { return nil }
	}

	err = enumerate(ctx, func(obj *database.Object) error {
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
	visited := map[string]bool{}
	for _, legacy := range slices.Sorted(maps.Keys(moves)) {
		move := moves[legacy]
		if !move.busy {
			// An operation that started after the resource was enumerated may resolve the legacy name before the
			// move. Read the resource again right before the move to keep that window small.
			move.busy, err = m.ownerBusy(ctx, db, move.owner)
			if errors.Is(err, &database.ErrNotFound{}) {
				// The resource was deleted, and its deletion removes the state. A lock this migrator kept is resolved
				// below.
				continue
			} else if err != nil {
				visited[legacy] = true
				result.Remaining++
				result.Message = err.Error()
				continue
			}
		}

		visited[legacy] = true
		if move.busy {
			if held, err := m.holdsLock(ctx, legacy); err == nil && held {
				// An earlier pass of this migrator kept the lock of the state, so an operation that resolved the legacy
				// name waits for it. Commit or remove the copy now, which ends the wait, rather than skip the move.
				moved := m.moveState(ctx, legacy, move.current, func(context.Context) (bool, error) { return true, nil })
				result.Add(moved)
				if moved.Message != "" {
					result.Message = moved.Message
				}
				continue
			}

			result.Remaining++
			result.Message = fmt.Sprintf("an operation is in progress on the resource that owns the Terraform state in %s", stateSecretName(legacy, 0))
			continue
		}

		moved := m.moveState(ctx, legacy, move.current, func(ctx context.Context) (bool, error) {
			return m.ownerBusy(ctx, db, move.owner)
		})
		result.Add(moved)
		if moved.Message != "" {
			result.Message = moved.Message
		}
	}

	// A state whose lock an earlier pass kept is resolved even if its resource was deleted since, or the lock would
	// block a resource created again with the same name. The copy is not published for a resource that is gone.
	kept := m.keptMoves()
	for _, legacy := range slices.Sorted(maps.Keys(kept)) {
		if visited[legacy] {
			continue
		}

		moved := m.moveState(ctx, legacy, kept[legacy], func(context.Context) (bool, error) { return true, nil })
		result.Add(moved)
		if moved.Message != "" {
			result.Message = moved.Message
		}
	}

	return result, nil
}

// holdsLock reports whether this migrator holds the lock of the state stored under suffix.
func (m *StateMigrator) holdsLock(ctx context.Context, suffix string) (bool, error) {
	lease, err := m.getLease(ctx, stateLockName(suffix))
	if apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity == m.holder, nil
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
	return m.moveState(ctx, legacySuffix, currentSuffix, nil)
}

// moveState moves a state. ownerBusy, if set, reports whether an operation started on the resource that owns the
// state. It is checked again right before the copy is published, because an operation that already resolved the
// legacy name would otherwise wait for the tombstone and fail.
func (m *StateMigrator) moveState(ctx context.Context, legacySuffix string, currentSuffix string, ownerBusy func(context.Context) (bool, error)) hashmigration.Result {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("legacySecret", stateSecretName(legacySuffix, 0), "currentSecret", stateSecretName(currentSuffix, 0))

	lock, err := m.lock(ctx, legacySuffix, currentSuffix)
	if err != nil {
		logger.Info("Unable to lock the Terraform state; the move will be retried", "reason", err.Error())
		return hashmigration.Result{Remaining: 1, Message: err.Error()}
	}

	result, outcome, err := m.moveLocked(ctx, lock, legacySuffix, currentSuffix, ownerBusy)
	switch {
	case errors.Is(err, errLockLost):
		// The migrator that took over the lock finishes the move, so this one must not write again.
		logger.Info("Another migrator took over the Terraform state lock; the move continues there", "reason", err.Error())
		m.forget(legacySuffix)
		return hashmigration.Result{Remaining: 1, Message: err.Error()}
	case outcome == lockHold:
		m.keep(legacySuffix, currentSuffix)
		// The lock stays held, so the legacy state cannot change under a copy that is or may become published, until a
		// later pass of this migrator, or of one that takes over the expired lock, commits or removes the copy. Terraform
		// ignores the lock duration, so the state is remaining work whatever this pass found, and the step is retried.
		message := result.Message
		if err != nil {
			message = err.Error()
		}
		if message == "" {
			message = fmt.Sprintf("the move of the Terraform state in %s is not finished", stateSecretName(legacySuffix, 0))
		}
		logger.Info("The Terraform state stays locked until a later pass finishes the move", "reason", message)
		return hashmigration.Result{Remaining: 1, Message: message}
	case errors.Is(err, errUnexpectedState):
		logger.Info("The Terraform state cannot be moved automatically", "reason", err.Error())
		result = hashmigration.Result{Skipped: 1, Message: err.Error()}
	case err != nil:
		logger.Error(err, "Unable to move the Terraform state; the move will be retried")
		result = hashmigration.Result{Remaining: 1, Message: err.Error()}
	case result.Conflicts > 0:
		logger.Info("The Terraform state is stored under both names with different content; Radius uses the current name and leaves the legacy state in place")
	case result.Migrated > 0 || result.Deduplicated > 0:
		logger.Info("Moved the Terraform state")
	case result.Remaining > 0:
		logger.Info("The Terraform state move was abandoned; it will be retried", "reason", result.Message)
	}

	if err := m.unlock(ctx, lock, legacySuffix, currentSuffix, outcome == lockTombstone); err != nil {
		if outcome == lockTombstone {
			// The state is stored under its current name, and Terraform cannot acquire the lock this migrator still holds.
			logger.Error(err, "Unable to turn the Terraform state lock into a tombstone")
			m.forget(legacySuffix)
			return result
		}

		// The lock stays held by this migrator, and Terraform ignores the lock duration, so a later pass re-acquires the
		// lock and releases it.
		logger.Error(err, "Unable to release the Terraform state lock; it will be retried")
		m.keep(legacySuffix, currentSuffix)
		result.Remaining = 1
		result.Message = fmt.Sprintf("unable to release the lock of the Terraform state in %s: %s", stateSecretName(legacySuffix, 0), err.Error())
		return result
	}

	m.forget(legacySuffix)
	return result
}

// lockOutcome is what a move leaves the lock of the legacy state as.
type lockOutcome int

const (
	// lockRelease releases the lock the way Terraform does.
	lockRelease lockOutcome = iota

	// lockTombstone keeps the lock as a tombstone because the state is stored under its current name.
	lockTombstone

	// lockHold keeps the lock held by the migrator because a copy may be published but its move is not committed.
	lockHold
)

// moveLocked moves the state while the migrator holds the lock of the legacy state.
func (m *StateMigrator) moveLocked(ctx context.Context, lock *stateLock, legacySuffix string, currentSuffix string, ownerBusy func(context.Context) (bool, error)) (hashmigration.Result, lockOutcome, error) {
	resting := lock.restingOutcome()
	legacy, err := m.listChunks(ctx, legacySuffix)
	if err != nil {
		return hashmigration.Result{}, lock.failedReadOutcome(err), err
	}

	current, err := m.listChunks(ctx, currentSuffix)
	if err != nil {
		return hashmigration.Result{}, lock.failedReadOutcome(err), err
	}

	first, published := current[0]
	if len(legacy) == 0 {
		// Moved or deleted since the Secrets were listed.
		if published {
			return hashmigration.Result{}, lockTombstone, nil
		}
		return hashmigration.Result{}, resting, nil
	}

	legacyChunks, ok := orderedChunks(legacy)
	if !ok {
		return hashmigration.Result{}, resting, fmt.Errorf("%w: the state stored under %s has missing parts", errUnexpectedState, stateSecretName(legacySuffix, 0))
	}

	if copied := lock.copiedParts(); len(copied) >= len(legacyChunks) && slices.Equal(stateParts(legacyChunks), copied[:len(legacyChunks)]) {
		// A move was committed and stopped before it deleted all of the legacy state. Terraform cannot write the legacy
		// state after the commit, so the Secrets that remain are copied versions and only need to be deleted.
		if err := m.deleteChunks(ctx, legacyChunks); err != nil {
			return hashmigration.Result{}, lockTombstone, err
		}
		return hashmigration.Result{Migrated: 1}, lockTombstone, nil
	}

	if published {
		result := hashmigration.Result{}
		currentChunks, complete := orderedChunks(current)
		switch {
		case first.Annotations[AnnotationStateMigratedParts] == encodeStateParts(stateParts(legacyChunks)):
			// An earlier pass published this copy of the legacy state and stopped before it committed the move.
			// Deployments have used the copy since it was published, so only the legacy state is left to delete.
			result.Migrated = 1
		case complete && sameState(currentChunks, legacyChunks):
			// The same state is stored under both names.
			result.Deduplicated = 1
		default:
			return hashmigration.Result{Conflicts: 1}, resting, nil
		}

		return m.commit(ctx, lock, legacyChunks, result)
	}

	return m.copyAndCommit(ctx, lock, legacyChunks, current, currentSuffix, ownerBusy)
}

// copyAndCommit copies the legacy state to its current name, publishes the copy, and commits the move. Deployments use
// the current name as soon as its first Secret exists, so that Secret is created last, once every other part is in
// place. It records the versions of the legacy Secrets the copy was made from. partial holds the Secrets of an
// unpublished copy that an interrupted move left.
func (m *StateMigrator) copyAndCommit(ctx context.Context, lock *stateLock, legacy []corev1.Secret, partial map[int]corev1.Secret, currentSuffix string, ownerBusy func(context.Context) (bool, error)) (hashmigration.Result, lockOutcome, error) {
	for index, existing := range partial {
		if index >= len(legacy) || existing.Annotations[AnnotationStateMigratedFromUID] != string(legacy[index].UID) {
			return hashmigration.Result{}, lock.restingOutcome(), fmt.Errorf("%w: %s exists but was not copied from %s", errUnexpectedState, existing.Name, legacy[min(index, len(legacy)-1)].Name)
		}
	}

	for index := len(legacy) - 1; index > 0; index-- {
		if err := m.renew(ctx, lock); err != nil {
			return hashmigration.Result{}, lockHold, err
		}

		copied := stateCopy(legacy[index], currentSuffix, index)
		var err error
		if existing, ok := partial[index]; ok {
			copied.ResourceVersion = existing.ResourceVersion
			_, err = m.updateSecret(ctx, copied)
		} else {
			_, err = m.createSecret(ctx, copied)
		}

		if err != nil {
			return m.abandonCopy(ctx, lock, legacy, currentSuffix, fmt.Errorf("failed to copy %s to %s: %w", legacy[index].Name, copied.Name, err))
		}
	}

	if ownerBusy != nil {
		busy, err := ownerBusy(ctx)
		if errors.Is(err, &database.ErrNotFound{}) {
			// The resource was deleted. Its deletion uses the legacy state, so the copy must not be published.
			busy, err = true, nil
		}

		if err != nil {
			return m.abandonCopy(ctx, lock, legacy, currentSuffix, err)
		} else if busy {
			_, outcome, err := m.abandonCopy(ctx, lock, legacy, currentSuffix, nil)
			message := fmt.Sprintf("an operation started on the resource that owns the Terraform state in %s", legacy[0].Name)
			return hashmigration.Result{Remaining: 1, Message: message}, outcome, err
		}
	}

	if err := m.renew(ctx, lock); err != nil {
		return hashmigration.Result{}, lockHold, err
	}

	parts := encodeStateParts(stateParts(legacy))
	first := stateCopy(legacy[0], currentSuffix, 0)
	first.Annotations[AnnotationStateMigratedParts] = parts
	if _, err := m.createSecret(ctx, first); err != nil {
		createErr := fmt.Errorf("failed to copy %s to %s: %w", legacy[0].Name, first.Name, err)
		published, checkErr := m.publishedCopy(ctx, first.Name, parts)
		switch {
		case published:
			// The create succeeded although its response was lost.
		case checkErr == nil && rejected(err):
			return m.abandonCopy(ctx, lock, legacy, currentSuffix, createErr)
		case errors.Is(checkErr, errUnexpectedState):
			return hashmigration.Result{}, lockHold, checkErr
		default:
			// The create may still be applied, so the copy must not be removed while it can become published.
			m.markUnsettled(lock.lease.Name)
			return hashmigration.Result{}, lockHold, createErr
		}
	}

	return m.commit(ctx, lock, legacy, hashmigration.Result{Migrated: 1})
}

// abandonCopy deletes the Secrets of an unpublished copy and returns the lock to its resting state, joining cause with
// any error. The first Secret of the copy does not exist when it is called, so no deployment uses the copy. If the
// copy cannot be deleted, the lock stays held so the legacy state cannot change under the remaining Secrets.
func (m *StateMigrator) abandonCopy(ctx context.Context, lock *stateLock, legacy []corev1.Secret, currentSuffix string, cause error) (hashmigration.Result, lockOutcome, error) {
	if lock.unsettled {
		// The first Secret that an earlier pass created with an uncertain outcome may still be applied and publish this
		// copy, so the copy and the lock are kept until that can no longer happen.
		return hashmigration.Result{}, lockHold, cause
	}

	for index := len(legacy) - 1; index > 0; index-- {
		if err := m.renew(ctx, lock); err != nil {
			return hashmigration.Result{}, lockHold, errors.Join(cause, err)
		}

		err := m.deleteSecret(ctx, stateSecretName(currentSuffix, index), nil)
		if err != nil && !apierrors.IsNotFound(err) {
			return hashmigration.Result{}, lockHold, errors.Join(cause, fmt.Errorf("failed to delete the unpublished copy %s: %w", stateSecretName(currentSuffix, index), err))
		}
	}

	return hashmigration.Result{}, lock.restingOutcome(), cause
}

// commit records on the lock that the state is stored under its current name, then deletes the legacy state. The
// record lets a later pass finish an interrupted delete, because Terraform cannot write the legacy state after the
// commit and the Secrets that remain are known to be copied.
func (m *StateMigrator) commit(ctx context.Context, lock *stateLock, legacy []corev1.Secret, result hashmigration.Result) (hashmigration.Result, lockOutcome, error) {
	lease := lock.lease.DeepCopy()
	now := metav1.NewMicroTime(m.now())
	lease.Spec.RenewTime = &now
	if lease.Labels == nil {
		lease.Labels = map[string]string{}
	}
	lease.Labels[LabelStateMigration] = "tombstone"
	if lease.Annotations == nil {
		lease.Annotations = map[string]string{}
	}
	lease.Annotations[AnnotationStateMigratedParts] = encodeStateParts(stateParts(legacy))

	updated, err := m.updateLease(ctx, lease)
	if err != nil {
		// Deployments use the published copy. The legacy state stays locked until a later pass commits the move.
		return hashmigration.Result{}, lockHold, lockError(err)
	}
	lock.lease = updated
	m.clearUnsettled(lock.lease.Name)

	if err := m.deleteChunks(ctx, legacy); err != nil {
		return hashmigration.Result{}, lockTombstone, err
	}

	return result, lockTombstone, nil
}

// publishedCopy reports whether the first Secret of a copy exists and records exactly the legacy versions in parts, as
// it does after a create whose response was lost.
func (m *StateMigrator) publishedCopy(ctx context.Context, name string, parts string) (bool, error) {
	secret, err := m.getSecret(ctx, name)
	if apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	if secret.Annotations[AnnotationStateMigratedParts] != parts {
		return false, fmt.Errorf("%w: %s was created by another writer during the move", errUnexpectedState, name)
	}

	return true, nil
}

// rejected reports whether the API server certainly did not apply a write that failed with err. Other failures, such
// as timeouts and lost connections, can be reported for writes that were applied.
func rejected(err error) bool {
	return apierrors.IsAlreadyExists(err) || apierrors.IsConflict(err) || apierrors.IsInvalid(err) ||
		apierrors.IsBadRequest(err) || apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) ||
		apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err) || apierrors.IsRequestEntityTooLargeError(err) ||
		apierrors.IsTooManyRequests(err) || apierrors.IsNotAcceptable(err) || apierrors.IsUnsupportedMediaType(err)
}

// renew extends the lock before the migrator writes. Every write of a move follows a renewal, so a migrator that takes
// over an expired lock never writes concurrently with the migrator it replaced, and a migrator that lost the lock stops.
func (m *StateMigrator) renew(ctx context.Context, lock *stateLock) error {
	lease := lock.lease.DeepCopy()
	now := metav1.NewMicroTime(m.now())
	lease.Spec.RenewTime = &now
	updated, err := m.updateLease(ctx, lease)
	if err != nil {
		return lockError(err)
	}

	lock.lease = updated
	return nil
}

// lockError marks a lock update that failed because another writer changed or removed the lock.
func lockError(err error) error {
	if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
		return fmt.Errorf("%w: %w", errLockLost, err)
	}

	return err
}

// deleteChunks deletes a legacy state. The first Secret is deleted last, so an interrupted delete leaves a state that
// a later pass recognizes and finishes deleting.
func (m *StateMigrator) deleteChunks(ctx context.Context, chunks []corev1.Secret) error {
	for index := len(chunks) - 1; index >= 0; index-- {
		chunk := chunks[index]
		err := m.deleteSecret(ctx, chunk.Name, &metav1.Preconditions{UID: &chunk.UID, ResourceVersion: &chunk.ResourceVersion})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete %s: %w", chunk.Name, err)
		}
	}

	return nil
}

// The following calls bound each Kubernetes request with stateMigrationCallTimeout.

func (m *StateMigrator) getSecret(ctx context.Context, name string) (*corev1.Secret, error) {
	ctx, cancel := context.WithTimeout(ctx, stateMigrationCallTimeout)
	defer cancel()
	return m.client.CoreV1().Secrets(m.namespace).Get(ctx, name, metav1.GetOptions{})
}

func (m *StateMigrator) createSecret(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error) {
	ctx, cancel := context.WithTimeout(ctx, stateMigrationCallTimeout)
	defer cancel()
	return m.client.CoreV1().Secrets(m.namespace).Create(ctx, secret, metav1.CreateOptions{})
}

func (m *StateMigrator) updateSecret(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error) {
	ctx, cancel := context.WithTimeout(ctx, stateMigrationCallTimeout)
	defer cancel()
	return m.client.CoreV1().Secrets(m.namespace).Update(ctx, secret, metav1.UpdateOptions{})
}

func (m *StateMigrator) deleteSecret(ctx context.Context, name string, preconditions *metav1.Preconditions) error {
	ctx, cancel := context.WithTimeout(ctx, stateMigrationCallTimeout)
	defer cancel()
	return m.client.CoreV1().Secrets(m.namespace).Delete(ctx, name, metav1.DeleteOptions{Preconditions: preconditions})
}

func (m *StateMigrator) getLease(ctx context.Context, name string) (*coordinationv1.Lease, error) {
	ctx, cancel := context.WithTimeout(ctx, stateMigrationCallTimeout)
	defer cancel()
	return m.client.CoordinationV1().Leases(m.namespace).Get(ctx, name, metav1.GetOptions{})
}

func (m *StateMigrator) createLease(ctx context.Context, lease *coordinationv1.Lease) (*coordinationv1.Lease, error) {
	ctx, cancel := context.WithTimeout(ctx, stateMigrationCallTimeout)
	defer cancel()
	return m.client.CoordinationV1().Leases(m.namespace).Create(ctx, lease, metav1.CreateOptions{})
}

func (m *StateMigrator) updateLease(ctx context.Context, lease *coordinationv1.Lease) (*coordinationv1.Lease, error) {
	ctx, cancel := context.WithTimeout(ctx, stateMigrationCallTimeout)
	defer cancel()
	return m.client.CoordinationV1().Leases(m.namespace).Update(ctx, lease, metav1.UpdateOptions{})
}

func (m *StateMigrator) listSecrets(ctx context.Context, options metav1.ListOptions) (*corev1.SecretList, error) {
	ctx, cancel := context.WithTimeout(ctx, stateMigrationCallTimeout)
	defer cancel()
	return m.client.CoreV1().Secrets(m.namespace).List(ctx, options)
}

// stateLock is a Terraform state lock held by the migrator.
type stateLock struct {
	lease *coordinationv1.Lease

	// wasTombstone records that the lock was a tombstone before the migrator acquired it.
	wasTombstone bool

	// inherited records that a migrator held the lock before this pass: this migrator in an earlier pass, or one that
	// stopped. It may have left a copy that is published, or may become published, without committing the move.
	inherited bool

	// unsettled records that the first Secret of a copy that this migrator created with an uncertain outcome may still
	// be applied.
	unsettled bool
}

// restingOutcome is what the lock returns to when a pass leaves the legacy state in place.
func (l *stateLock) restingOutcome() lockOutcome {
	switch {
	case l.wasTombstone:
		return lockTombstone
	case l.unsettled:
		return lockHold
	default:
		return lockRelease
	}
}

// failedReadOutcome is what the lock returns to when a pass cannot read the state. If a migrator held the lock before
// this pass, the state may have a copy that it did not commit, so the lock stays held until a later pass reads the
// state. A state that was read and found unexpected is left as it is.
func (l *stateLock) failedReadOutcome(err error) lockOutcome {
	if l.inherited && !l.wasTombstone && !errors.Is(err, errUnexpectedState) {
		return lockHold
	}

	return l.restingOutcome()
}

// copiedParts returns the versions of the legacy Secrets that a committed move copied, or nil if no move was committed.
func (l *stateLock) copiedParts() []statePart {
	value := l.lease.Annotations[AnnotationStateMigratedParts]
	if value == "" {
		return nil
	}

	parts := []statePart{}
	if err := json.Unmarshal([]byte(value), &parts); err != nil {
		return nil
	}

	return parts
}

// lock acquires the Terraform lock of the legacy state the way Terraform does, so deployments and the migrator
// exclude each other.
func (m *StateMigrator) lock(ctx context.Context, legacySuffix string, currentSuffix string) (*stateLock, error) {
	name := stateLockName(legacySuffix)
	info := fmt.Sprintf("Radius is moving this Terraform state to Kubernetes Secret %s to migrate state names from SHA-1 to SHA-256.", stateSecretName(currentSuffix, 0))

	lease, err := m.getLease(ctx, name)
	if apierrors.IsNotFound(err) {
		lease = &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: m.namespace, Labels: stateLabels(legacySuffix)}}
		if err := m.hold(lease, m.holder, info); err != nil {
			return nil, err
		}

		created, err := m.createLease(ctx, lease)
		if apierrors.IsAlreadyExists(err) {
			return nil, errStateLocked
		} else if err != nil {
			return nil, err
		}

		return &stateLock{lease: created, unsettled: m.isUnsettled(name)}, nil
	} else if err != nil {
		return nil, err
	}

	holder := ""
	if lease.Spec.HolderIdentity != nil {
		holder = *lease.Spec.HolderIdentity
	}

	// The label outlives a migrator holding the tombstone, for example to finish deleting a moved state.
	wasTombstone := holder == StateMigrationTombstone || lease.Labels[LabelStateMigration] == "tombstone"
	switch {
	case holder == "", holder == StateMigrationTombstone, holder == m.holder:
	case strings.HasPrefix(holder, StateMigrationHolderPrefix) && m.expired(lease):
		// A migrator stopped while moving the state. moveLocked finishes or restarts its move.
	default:
		return nil, fmt.Errorf("%w by %s", errStateLocked, holder)
	}

	if err := m.hold(lease, m.holder, info); err != nil {
		return nil, err
	}

	updated, err := m.updateLease(ctx, lease)
	if apierrors.IsConflict(err) {
		return nil, errStateLocked
	} else if err != nil {
		return nil, err
	}

	return &stateLock{
		lease:        updated,
		wasTombstone: wasTombstone,
		inherited:    holder != StateMigrationTombstone && strings.HasPrefix(holder, StateMigrationHolderPrefix),
		unsettled:    holder == m.holder && m.isUnsettled(name),
	}, nil
}

// unlock releases the lock. When tombstone is set, the lock becomes a tombstone that Terraform cannot acquire.
// Otherwise it is released the way Terraform releases it.
func (m *StateMigrator) unlock(ctx context.Context, lock *stateLock, legacySuffix string, currentSuffix string, tombstone bool) error {
	lease := lock.lease.DeepCopy()
	if tombstone {
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

	_, err := m.updateLease(ctx, lease)
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
		list, err := m.listSecrets(ctx, options)
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
	list, err := m.listSecrets(ctx, metav1.ListOptions{LabelSelector: stateSelector(suffix)})
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

// statePart identifies one version of a Secret that holds part of a state.
type statePart struct {
	UID             types.UID `json:"uid"`
	ResourceVersion string    `json:"resourceVersion"`
}

// stateParts returns the versions of the Secrets of a state, in part order.
func stateParts(chunks []corev1.Secret) []statePart {
	parts := make([]statePart, 0, len(chunks))
	for _, chunk := range chunks {
		parts = append(parts, statePart{UID: chunk.UID, ResourceVersion: chunk.ResourceVersion})
	}

	return parts
}

// encodeStateParts returns the AnnotationStateMigratedParts value that records parts.
func encodeStateParts(parts []statePart) string {
	// Encoding a slice of string fields cannot fail.
	b, _ := json.Marshal(parts)
	return string(b)
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

	// Only the first Secret of a copy records the parts it was copied from, and only once the copy is published.
	delete(copied.Annotations, AnnotationStateMigratedParts)

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
