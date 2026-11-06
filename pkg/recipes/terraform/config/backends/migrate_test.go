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
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
)

const (
	migrateEnvironmentID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/env"
	migrateApplicationID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/app"
	migrateResourcePath  = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Datastores/redisCaches/"
)

type stateFixture struct {
	t        *testing.T
	client   *fake.Clientset
	db       *inmemory.Client
	migrator *StateMigrator
	now      time.Time
}

func newStateFixture(t *testing.T) *stateFixture {
	f := &stateFixture{t: t, client: fake.NewClientset(), db: inmemory.NewClient(), now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	f.migrator = NewStateMigrator(f.client)
	f.migrator.now = func() time.Time { return f.now }
	return f
}

// addResource stores an idle resource and returns the current and legacy suffixes of its state.
func (f *stateFixture) addResource(name string, application string) (string, string) {
	return f.addResourceInState(name, application, "Succeeded")
}

// addResourceInState stores a resource in the given provisioning state and returns the current and legacy suffixes
// of its state.
func (f *stateFixture) addResourceInState(name string, application string, state string) (string, string) {
	id := migrateResourcePath + name
	properties := map[string]any{"environment": migrateEnvironmentID}
	if application != "" {
		properties["application"] = application
	}
	data := map[string]any{"id": id, "properties": properties, "provisioningState": state}
	require.NoError(f.t, f.db.Save(f.t.Context(), &database.Object{ID: id, Data: data}))

	current, legacy, err := SecretSuffixes(&recipes.ResourceMetadata{ResourceID: id, EnvironmentID: migrateEnvironmentID, ApplicationID: application})
	require.NoError(f.t, err)
	return current, legacy
}

// addState stores a state the way the Terraform Kubernetes backend does, one Secret per part.
func (f *stateFixture) addState(suffix string, parts ...string) []corev1.Secret {
	secrets := []corev1.Secret{}
	for index, part := range parts {
		name := stateSecretName(suffix, index)
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   RadiusNamespace,
				UID:         types.UID("uid-" + name),
				Labels:      stateLabels(suffix),
				Annotations: map[string]string{"encoding": "gzip"},
			},
			Data: map[string][]byte{tfstateDataKey: []byte(part)},
		}
		created, err := f.client.CoreV1().Secrets(RadiusNamespace).Create(f.t.Context(), secret, metav1.CreateOptions{})
		require.NoError(f.t, err)
		secrets = append(secrets, *created)
	}
	return secrets
}

// state returns the parts of the state stored under suffix.
func (f *stateFixture) state(suffix string) []string {
	chunks, err := f.migrator.listChunks(f.t.Context(), suffix)
	require.NoError(f.t, err)
	ordered, ok := orderedChunks(chunks)
	require.True(f.t, ok, "the state must have every part")
	parts := []string{}
	for _, chunk := range ordered {
		parts = append(parts, string(chunk.Data[tfstateDataKey]))
	}
	return parts
}

func (f *stateFixture) setLease(suffix string, holder *string, renewed time.Time) {
	f.setMigrationLease(suffix, "", holder, renewed)
}

// setMigrationLease creates the lock of the state stored under suffix. A lock held by a migrator records where the
// state moves, current, as the migrator writes it.
func (f *stateFixture) setMigrationLease(suffix string, current string, holder *string, renewed time.Time) {
	duration := int32(stateMigrationLeaseDuration.Seconds())
	renewTime := metav1.NewMicroTime(renewed)
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: stateLockName(suffix), Namespace: RadiusNamespace, Labels: stateLabels(suffix)},
		Spec:       coordinationv1.LeaseSpec{HolderIdentity: holder, RenewTime: &renewTime, LeaseDurationSeconds: &duration},
	}
	if holder != nil {
		lease.Annotations = map[string]string{tfstateLockInfoAnnotation: `{"ID":"` + *holder + `","Operation":"OperationTypeApply","Who":"terraform"}`}
		if current != "" {
			lease.Annotations[AnnotationStateMigratedTo] = current
		}
	}
	_, err := f.client.CoordinationV1().Leases(RadiusNamespace).Create(f.t.Context(), lease, metav1.CreateOptions{})
	require.NoError(f.t, err)
}

func (f *stateFixture) lease(suffix string) *coordinationv1.Lease {
	lease, err := f.client.CoordinationV1().Leases(RadiusNamespace).Get(f.t.Context(), stateLockName(suffix), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	require.NoError(f.t, err)
	return lease
}

func (f *stateFixture) requireTombstone(legacy string, current string) {
	lease := f.lease(legacy)
	require.NotNil(f.t, lease)
	require.Equal(f.t, StateMigrationTombstone, *lease.Spec.HolderIdentity)
	require.Equal(f.t, "tombstone", lease.Labels[LabelStateMigration])
	require.Equal(f.t, "true", lease.Labels[tfstateLabel], "Terraform only deletes locks labeled as state")

	// Terraform prints the lock information when it cannot acquire the lock, so it must parse and name the new Secret.
	info := terraformLockInfo{}
	require.NoError(f.t, json.Unmarshal([]byte(lease.Annotations[tfstateLockInfoAnnotation]), &info))
	require.Equal(f.t, StateMigrationTombstone, info.ID)
	require.Contains(f.t, info.Info, stateSecretName(current, 0))
}

// requireHeld asserts that the migrator still holds the lock of the legacy state, as it does while a copy may be
// published but its move is not committed.
func (f *stateFixture) requireHeld(legacy string) {
	lease := f.lease(legacy)
	require.NotNil(f.t, lease)
	require.NotNil(f.t, lease.Spec.HolderIdentity)
	require.Equal(f.t, f.migrator.holder, *lease.Spec.HolderIdentity)
}

// publishCopy copies a legacy state to the current suffix the way a move does, creating the first Secret last with the
// versions of the legacy Secrets it was copied from.
func (f *stateFixture) publishCopy(legacySecrets []corev1.Secret, current string) {
	for index := len(legacySecrets) - 1; index >= 0; index-- {
		copied := stateCopy(legacySecrets[index], current, index)
		if index == 0 {
			copied.Annotations[AnnotationStateMigratedParts] = encodeStateParts(stateParts(legacySecrets))
		}
		_, err := f.client.CoreV1().Secrets(RadiusNamespace).Create(f.t.Context(), copied, metav1.CreateOptions{})
		require.NoError(f.t, err)
	}
}

// chunkNames returns the names of the Secrets stored under suffix, by part index, including those of an incomplete
// state.
func (f *stateFixture) chunkNames(suffix string) map[int]string {
	chunks, err := f.migrator.listChunks(f.t.Context(), suffix)
	require.NoError(f.t, err)
	names := map[int]string{}
	for index, chunk := range chunks {
		names[index] = chunk.Name
	}
	return names
}

func (f *stateFixture) requireFree(legacy string) {
	lease := f.lease(legacy)
	require.NotNil(f.t, lease)
	require.Nil(f.t, lease.Spec.HolderIdentity)
	require.NotContains(f.t, lease.Annotations, tfstateLockInfoAnnotation)
}

func (f *stateFixture) migrate() hashmigration.Result {
	result, err := f.migrator.MigrateLegacyStates(f.t.Context(), f.db)
	require.NoError(f.t, err)
	result.Message = ""
	return result
}

func Test_MigrateLegacyStates_Moves(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name        string
		application string
		parts       []string
	}{
		{name: "application resource", application: migrateApplicationID, parts: []string{"state"}},
		{name: "environment resource", parts: []string{"state"}},
		{name: "state in parts", application: migrateApplicationID, parts: []string{"part-0", "part-1", "part-2"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newStateFixture(t)
			current, legacy := f.addResource("redis", tt.application)
			legacySecrets := f.addState(legacy, tt.parts...)

			require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
			require.Equal(t, tt.parts, f.state(current))
			require.Empty(t, f.state(legacy))
			f.requireTombstone(legacy, current)

			for index, source := range legacySecrets {
				copied, err := f.client.CoreV1().Secrets(RadiusNamespace).Get(t.Context(), stateSecretName(current, index), metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, current, copied.Labels[tfstateSecretSuffixLabel])
				require.Equal(t, "true", copied.Labels[tfstateLabel])
				require.Equal(t, tfstateWorkspace, copied.Labels[tfstateWorkspaceLabel])
				require.Equal(t, "gzip", copied.Annotations["encoding"])
				require.Equal(t, source.Name, copied.Annotations[AnnotationStateMigratedFrom])
				require.Equal(t, string(source.UID), copied.Annotations[AnnotationStateMigratedFromUID])
				if index > 0 {
					require.NotContains(t, copied.Annotations, AnnotationStateMigratedParts, "only the first Secret records the copied parts")
				}
			}

			// The first Secret of the copy and the tombstone record exactly the legacy Secrets that were copied.
			parts := encodeStateParts(stateParts(legacySecrets))
			first, err := f.client.CoreV1().Secrets(RadiusNamespace).Get(t.Context(), stateSecretName(current, 0), metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, parts, first.Annotations[AnnotationStateMigratedParts])
			require.Equal(t, parts, f.lease(legacy).Annotations[AnnotationStateMigratedParts])

			// Deployments use the moved state.
			resolved, err := NewKubernetesBackend(f.client).(*kubernetesBackend).resolveSecretSuffix(&recipes.ResourceMetadata{
				ResourceID: migrateResourcePath + "redis", EnvironmentID: migrateEnvironmentID, ApplicationID: tt.application,
			})
			require.NoError(t, err)
			require.Equal(t, current, resolved)

			// The migration is idempotent.
			require.Equal(t, hashmigration.Result{}, f.migrate())
			require.Equal(t, tt.parts, f.state(current))
			f.requireTombstone(legacy, current)
		})
	}
}

func Test_MigrateLegacyStates_NothingToMove(t *testing.T) {
	t.Parallel()

	f := newStateFixture(t)
	current, legacy := f.addResource("current", migrateApplicationID)
	f.addState(current, "state")
	f.addResource("stateless", migrateApplicationID)
	require.NoError(t, f.db.Save(t.Context(), &database.Object{ID: migrateEnvironmentID, Data: map[string]any{"properties": map[string]any{}}}))

	require.Equal(t, hashmigration.Result{}, f.migrate())
	require.Equal(t, []string{"state"}, f.state(current))
	require.Nil(t, f.lease(legacy), "the migration must not lock states it does not move")
}

func Test_MigrateLegacyStates_OperationInProgress(t *testing.T) {
	t.Parallel()

	f := newStateFixture(t)
	current, legacy := f.addResourceInState("redis", migrateApplicationID, "Updating")
	f.addState(legacy, "state")

	// The operation may already have resolved the legacy name, so the state is moved once the operation finishes.
	require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
	require.Equal(t, []string{"state"}, f.state(legacy))
	require.Nil(t, f.lease(legacy), "the migration must not lock the state of a busy resource")

	f.addResourceInState("redis", migrateApplicationID, "Failed")
	require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
	require.Equal(t, []string{"state"}, f.state(current))
	f.requireTombstone(legacy, current)
}

// changingDatabase returns a different version of a resource when it is read again, as when an operation starts or the
// resource is deleted after the migration enumerated it.
type changingDatabase struct {
	*inmemory.Client
	get func(id string) (*database.Object, error)
}

func (d *changingDatabase) Get(_ context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
	return d.get(id)
}

func Test_MigrateLegacyStates_OwnerChangesBeforeMove(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		get      func(id string) (*database.Object, error)
		expected hashmigration.Result
	}{
		{
			name: "operation started",
			get: func(id string) (*database.Object, error) {
				data := map[string]any{"id": id, "provisioningState": "Accepted", "properties": map[string]any{"environment": migrateEnvironmentID}}
				return &database.Object{ID: id, Data: data}, nil
			},
			expected: hashmigration.Result{Remaining: 1},
		},
		{
			name:     "resource deleted",
			get:      func(id string) (*database.Object, error) { return nil, &database.ErrNotFound{ID: id} },
			expected: hashmigration.Result{},
		},
		{
			name:     "read fails",
			get:      func(string) (*database.Object, error) { return nil, errors.New("database unavailable") },
			expected: hashmigration.Result{Remaining: 1},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newStateFixture(t)
			current, legacy := f.addResource("redis", "")
			f.addState(legacy, "state")

			result, err := f.migrator.MigrateLegacyStates(t.Context(), &changingDatabase{Client: f.db, get: tt.get})
			require.NoError(t, err)
			result.Message = ""
			require.Equal(t, tt.expected, result)
			require.Equal(t, []string{"state"}, f.state(legacy), "the state must not be moved")
			require.Empty(t, f.state(current))
			require.Nil(t, f.lease(legacy), "the migration must not lock the state")
		})
	}
}

func Test_MigrateLegacyStates_OwnerChangesBeforePublication(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		owner func(id string) (*database.Object, error)
	}{
		{
			name: "operation started",
			owner: func(id string) (*database.Object, error) {
				data := map[string]any{"id": id, "provisioningState": "Deleting", "properties": map[string]any{"environment": migrateEnvironmentID}}
				return &database.Object{ID: id, Data: data}, nil
			},
		},
		{name: "resource deleted", owner: func(id string) (*database.Object, error) { return nil, &database.ErrNotFound{ID: id} }},
		{name: "read fails", owner: func(string) (*database.Object, error) { return nil, errors.New("database unavailable") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newStateFixture(t)
			current, legacy := f.addResource("redis", "")
			f.addState(legacy, "a", "b")

			// The resource is idle when the move starts and changes while the state is copied. An operation that resolved
			// the legacy name must find the legacy state, not a tombstone.
			reads := 0
			db := &changingDatabase{Client: f.db, get: func(id string) (*database.Object, error) {
				reads++
				if reads == 1 {
					return f.db.Get(t.Context(), id)
				}
				return tt.owner(id)
			}}

			result, err := f.migrator.MigrateLegacyStates(t.Context(), db)
			require.NoError(t, err)
			require.Equal(t, 2, reads)
			require.Equal(t, 1, result.Remaining)
			require.Zero(t, result.Migrated)
			require.Equal(t, []string{"a", "b"}, f.state(legacy))
			require.Empty(t, f.chunkNames(current), "the unpublished copy must be removed")
			f.requireFree(legacy)
		})
	}
}

func Test_MigrateLegacyStates_RequiresEnumerator(t *testing.T) {
	t.Parallel()
	_, err := NewStateMigrator(fake.NewClientset()).MigrateLegacyStates(t.Context(), plainDatabase{})
	require.ErrorContains(t, err, "cannot enumerate")
}

func Test_MigrateLegacyStates_UsesResourceDataID(t *testing.T) {
	t.Parallel()

	// A deployment computes the suffix from the ID in the resource data.
	f := newStateFixture(t)
	id := migrateResourcePath + "Redis"
	current, legacy, err := SecretSuffixes(&recipes.ResourceMetadata{ResourceID: id, EnvironmentID: migrateEnvironmentID})
	require.NoError(t, err)
	data := map[string]any{"id": id, "properties": map[string]any{"environment": migrateEnvironmentID}}
	require.NoError(t, f.db.Save(t.Context(), &database.Object{ID: migrateResourcePath + "stored-key", Data: data}))
	f.addState(legacy, "state")

	require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
	require.Equal(t, []string{"state"}, f.state(current))
}

type plainDatabase struct {
	database.Client
}

func Test_NewStateMigrationStep(t *testing.T) {
	t.Parallel()

	clientset := func() (kubernetes.Interface, error) { return fake.NewClientset(), nil }

	t.Run("migrates", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "state")
		step := NewStateMigrationStep(func() (kubernetes.Interface, error) { return f.client, nil }, func(context.Context) (database.Client, error) { return f.db, nil })
		require.Equal(t, StateMigrationStepName, step.Name())

		result, err := step.Run(t.Context())
		require.NoError(t, err)
		require.Equal(t, 1, result.Migrated)
		require.Equal(t, []string{"state"}, f.state(current))
	})

	t.Run("database cannot enumerate", func(t *testing.T) {
		t.Parallel()
		step := NewStateMigrationStep(clientset, func(context.Context) (database.Client, error) { return plainDatabase{}, nil })
		result, err := step.Run(t.Context())
		require.NoError(t, err)
		require.True(t, result.NotApplicable)
	})

	t.Run("database unavailable", func(t *testing.T) {
		t.Parallel()
		step := NewStateMigrationStep(clientset, func(context.Context) (database.Client, error) { return nil, errors.New("database unavailable") })
		_, err := step.Run(t.Context())
		require.ErrorContains(t, err, "database unavailable")
	})

	t.Run("cluster unavailable", func(t *testing.T) {
		t.Parallel()
		step := NewStateMigrationStep(func() (kubernetes.Interface, error) { return nil, errors.New("no cluster") }, func(context.Context) (database.Client, error) { return inmemory.NewClient(), nil })
		_, err := step.Run(t.Context())
		require.ErrorContains(t, err, "no cluster")
	})
}

func Test_MigrateLegacyStates_Locks(t *testing.T) {
	t.Parallel()

	t.Run("deployment holds the lock", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "state")
		f.setLease(legacy, new("terraform-run"), f.now)

		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		require.Equal(t, []string{"state"}, f.state(legacy))
		require.Empty(t, f.state(current))
		require.Equal(t, "terraform-run", *f.lease(legacy).Spec.HolderIdentity)
	})

	t.Run("released lock", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "state")
		f.setLease(legacy, nil, f.now)

		require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
		require.Equal(t, []string{"state"}, f.state(current))
		f.requireTombstone(legacy, current)
	})

	t.Run("another migrator holds the lock", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "state")
		f.setLease(legacy, new(StateMigrationHolderPrefix+"other"), f.now)

		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		require.Empty(t, f.state(current))

		// A migrator that stopped is taken over once its lock expires.
		f.now = f.now.Add(stateMigrationLeaseDuration + time.Second)
		require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
		require.Equal(t, []string{"state"}, f.state(current))
		f.requireTombstone(legacy, current)
	})

	t.Run("state restored under a moved name", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "restored")
		f.setLease(legacy, new(StateMigrationTombstone), f.now)

		require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
		require.Equal(t, []string{"restored"}, f.state(current))
		f.requireTombstone(legacy, current)
	})
}

func Test_MigrateLegacyStates_BothNames(t *testing.T) {
	t.Parallel()

	t.Run("same state", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b")
		f.addState(current, "a", "b")

		require.Equal(t, hashmigration.Result{Deduplicated: 1}, f.migrate())
		require.Empty(t, f.state(legacy))
		require.Equal(t, []string{"a", "b"}, f.state(current))
		f.requireTombstone(legacy, current)
	})

	t.Run("different states", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "legacy")
		f.addState(current, "current")

		require.Equal(t, hashmigration.Result{Conflicts: 1}, f.migrate())
		require.Equal(t, []string{"legacy"}, f.state(legacy), "a conflicting state must be left for review")
		require.Equal(t, []string{"current"}, f.state(current))
		f.requireFree(legacy)
	})

	t.Run("interrupted delete after the commit", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b", "c")
		failing := true
		f.client.PrependReactor("delete", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if failing && action.(k8stesting.DeleteAction).GetName() == stateSecretName(legacy, 1) {
				return true, nil, errors.New("delete failed")
			}
			return false, nil, nil
		})

		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		require.Equal(t, []string{"a", "b", "c"}, f.state(current))
		require.Equal(t, []string{"a", "b"}, f.state(legacy), "the legacy state is deleted from its last part")
		f.requireTombstone(legacy, current)

		failing = false
		require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
		require.Empty(t, f.state(legacy))
		require.Equal(t, []string{"a", "b", "c"}, f.state(current))
		f.requireTombstone(legacy, current)
	})

	t.Run("legacy state restored after the commit", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a")
		require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())

		// Secrets restored under the legacy name are not the copied versions, so they are not deleted.
		restored := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: stateSecretName(legacy, 0), Namespace: RadiusNamespace, UID: "restored", Labels: stateLabels(legacy)},
			Data:       map[string][]byte{tfstateDataKey: []byte("restored")},
		}
		_, err := f.client.CoreV1().Secrets(RadiusNamespace).Create(t.Context(), restored, metav1.CreateOptions{})
		require.NoError(t, err)
		require.Equal(t, hashmigration.Result{Conflicts: 1}, f.migrate())
		require.Equal(t, []string{"restored"}, f.state(legacy))
		require.Equal(t, []string{"a"}, f.state(current))
		f.requireTombstone(legacy, current)
	})

	t.Run("deployment changed a published copy before the commit", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b")
		failing := true
		f.client.PrependReactor("update", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
			lease := action.(k8stesting.UpdateAction).GetObject().(*coordinationv1.Lease)
			if failing && lease.Annotations[AnnotationStateMigratedParts] != "" {
				return true, nil, errors.New("connection reset")
			}
			return false, nil, nil
		})

		// The copy is published, but the outcome of the commit is unknown, so the legacy state stays locked.
		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		require.Equal(t, []string{"a", "b"}, f.state(current))
		require.Equal(t, []string{"a", "b"}, f.state(legacy))
		f.requireHeld(legacy)

		// A deployment uses the published copy. Terraform writes the state in place, keeping the copy's annotations.
		copied, err := f.client.CoreV1().Secrets(RadiusNamespace).Get(t.Context(), stateSecretName(current, 0), metav1.GetOptions{})
		require.NoError(t, err)
		copied.Data[tfstateDataKey] = []byte("deployed")
		_, err = f.client.CoreV1().Secrets(RadiusNamespace).Update(t.Context(), copied, metav1.UpdateOptions{})
		require.NoError(t, err)

		failing = false
		require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
		require.Empty(t, f.state(legacy), "the legacy state is what remains of the move")
		require.Equal(t, []string{"deployed", "b"}, f.state(current))
		f.requireTombstone(legacy, current)
	})

	t.Run("published copy of another version", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		legacySecrets := f.addState(legacy, "a", "b")

		// The copy records versions of the legacy Secrets that are no longer stored, so it cannot be trusted as complete.
		published := []corev1.Secret{*legacySecrets[0].DeepCopy(), *legacySecrets[1].DeepCopy()}
		published[1].ResourceVersion = "older"
		f.publishCopy(published, current)
		copiedPart, err := f.client.CoreV1().Secrets(RadiusNamespace).Get(t.Context(), stateSecretName(current, 1), metav1.GetOptions{})
		require.NoError(t, err)
		copiedPart.Data[tfstateDataKey] = []byte("truncated")
		_, err = f.client.CoreV1().Secrets(RadiusNamespace).Update(t.Context(), copiedPart, metav1.UpdateOptions{})
		require.NoError(t, err)

		require.Equal(t, hashmigration.Result{Conflicts: 1}, f.migrate())
		require.Equal(t, []string{"a", "b"}, f.state(legacy))
	})

	t.Run("legacy state changed after the copy", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		legacySecrets := f.addState(legacy, "a")
		f.publishCopy(legacySecrets, current)

		// A writer that ignored the lock changed the legacy state, so it is no longer the version that was copied.
		changed := legacySecrets[0].DeepCopy()
		changed.ResourceVersion = "changed"
		changed.Data[tfstateDataKey] = []byte("changed")
		_, err := f.client.CoreV1().Secrets(RadiusNamespace).Update(t.Context(), changed, metav1.UpdateOptions{})
		require.NoError(t, err)

		require.Equal(t, hashmigration.Result{Conflicts: 1}, f.migrate())
		require.Equal(t, []string{"changed"}, f.state(legacy))
		require.Equal(t, []string{"a"}, f.state(current))
	})

	t.Run("interrupted copy", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		legacySecrets := f.addState(legacy, "a", "b", "c")
		partial := stateCopy(legacySecrets[2], current, 2)
		partial.Data[tfstateDataKey] = []byte("stale")
		_, err := f.client.CoreV1().Secrets(RadiusNamespace).Create(t.Context(), partial, metav1.CreateOptions{})
		require.NoError(t, err)

		require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
		require.Equal(t, []string{"a", "b", "c"}, f.state(current))
		require.Empty(t, f.state(legacy))
	})
}

func Test_MigrateLegacyStates_Failures(t *testing.T) {
	t.Parallel()

	t.Run("rejected copy is removed", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b")
		f.client.PrependReactor("create", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if action.(k8stesting.CreateAction).GetObject().(*corev1.Secret).Name == stateSecretName(current, 0) {
				return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, stateSecretName(current, 0), errors.New("denied"))
			}
			return false, nil, nil
		})

		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		require.Equal(t, []string{"a", "b"}, f.state(legacy))
		require.Empty(t, f.chunkNames(current), "an unpublished copy must be removed")
		f.requireFree(legacy)
	})

	t.Run("copy of a part fails", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b", "c")
		f.client.PrependReactor("create", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if action.(k8stesting.CreateAction).GetObject().(*corev1.Secret).Name == stateSecretName(current, 1) {
				return true, nil, errors.New("connection reset")
			}
			return false, nil, nil
		})

		// The first Secret was never created, so the parts already copied can be removed even after an uncertain failure.
		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		require.Equal(t, []string{"a", "b", "c"}, f.state(legacy))
		require.Empty(t, f.chunkNames(current))
		f.requireFree(legacy)
	})

	for _, tt := range []struct {
		name string

		// applied records that the API server created the first Secret although the create failed.
		applied bool

		// appliedLater creates the first Secret after the migrator checked for it, as a delayed request does.
		appliedLater bool
	}{
		{name: "uncertain copy was created", applied: true},
		{name: "uncertain copy was not created"},
		{name: "uncertain copy is created later", appliedLater: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newStateFixture(t)
			current, legacy := f.addResource("redis", migrateApplicationID)
			legacySecrets := f.addState(legacy, "a", "b", "c")
			failing := true
			f.client.PrependReactor("create", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
				secret := action.(k8stesting.CreateAction).GetObject().(*corev1.Secret)
				if !failing || secret.Name != stateSecretName(current, 0) {
					return false, nil, nil
				}
				if tt.applied {
					require.NoError(t, f.client.Tracker().Create(corev1.SchemeGroupVersion.WithResource("secrets"), secret.DeepCopy(), RadiusNamespace))
				}
				return true, nil, apierrors.NewTimeoutError("the request timed out", 1)
			})

			if tt.applied {
				// The migrator finds the copy it published and completes the move.
				require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
				require.Equal(t, []string{"a", "b", "c"}, f.state(current))
				require.Empty(t, f.state(legacy))
				f.requireTombstone(legacy, current)
				return
			}

			// The create may still be applied, so the copy is kept and the legacy state stays locked: a deployment that
			// resolved the legacy name cannot change it under the copy.
			require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
			require.Equal(t, []string{"a", "b", "c"}, f.state(legacy))
			require.Equal(t, map[int]string{1: stateSecretName(current, 1), 2: stateSecretName(current, 2)}, f.chunkNames(current))
			f.requireHeld(legacy)

			failing = false
			if tt.appliedLater {
				_, err := f.client.CoreV1().Secrets(RadiusNamespace).Get(t.Context(), stateSecretName(current, 0), metav1.GetOptions{})
				require.True(t, apierrors.IsNotFound(err))
				first := stateCopy(legacySecrets[0], current, 0)
				first.Annotations[AnnotationStateMigratedParts] = encodeStateParts(stateParts(legacySecrets))
				_, err = f.client.CoreV1().Secrets(RadiusNamespace).Create(t.Context(), first, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
			require.Equal(t, []string{"a", "b", "c"}, f.state(current))
			require.Empty(t, f.state(legacy))
			f.requireTombstone(legacy, current)
		})
	}

	t.Run("lock lost during the move", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b")
		updates := 0
		f.client.PrependReactor("update", "leases", func(k8stesting.Action) (bool, runtime.Object, error) {
			updates++
			if updates >= 2 {
				// Another migrator took over the lock, which changed its resource version.
				return true, nil, apierrors.NewConflict(schema.GroupResource{Group: "coordination.k8s.io", Resource: "leases"}, stateLockName(legacy), errors.New("modified"))
			}
			return false, nil, nil
		})

		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		require.Equal(t, 2, updates, "a migrator that lost the lock must not write to it again")
		require.Equal(t, []string{"a", "b"}, f.state(legacy))
		require.Equal(t, map[int]string{1: stateSecretName(current, 1)}, f.chunkNames(current), "the new holder of the lock owns the copy")
		for _, action := range f.client.Actions() {
			require.False(t, action.Matches("delete", "secrets"), "a migrator that lost the lock must not delete Secrets")
		}
	})

	t.Run("delete fails", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b")
		failing := true
		f.client.PrependReactor("delete", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if failing && action.(k8stesting.DeleteAction).GetName() == stateSecretName(legacy, 0) {
				return true, nil, errors.New("delete failed")
			}
			return false, nil, nil
		})

		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		require.Equal(t, []string{"a", "b"}, f.state(current), "the copy is complete and used")
		require.Equal(t, []string{"a"}, f.state(legacy))
		f.requireTombstone(legacy, current)

		failing = false
		require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
		require.Empty(t, f.state(legacy))
		f.requireTombstone(legacy, current)
	})

	t.Run("unexpected state Secret", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a")
		_, err := f.client.CoreV1().Secrets(RadiusNamespace).Create(t.Context(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "unexpected", Namespace: RadiusNamespace, Labels: stateLabels(legacy)},
		}, metav1.CreateOptions{})
		require.NoError(t, err)

		require.Equal(t, hashmigration.Result{Skipped: 1}, f.migrate())
		require.Empty(t, f.state(current))
		f.requireFree(legacy)
	})

	t.Run("listing fails", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		f.client.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", errors.New("denied"))
		})

		_, err := f.migrator.MigrateLegacyStates(t.Context(), f.db)
		require.ErrorContains(t, err, "failed to list Terraform state Secrets")
	})
}

func Test_MigrateLegacyStates_HeldLocks(t *testing.T) {
	t.Parallel()

	// failFirstCreate makes the create of the first Secret of the copy time out without being applied while failing is set.
	failFirstCreate := func(f *stateFixture, current string, failing *bool) {
		f.client.PrependReactor("create", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if *failing && action.(k8stesting.CreateAction).GetObject().(*corev1.Secret).Name == stateSecretName(current, 0) {
				return true, nil, apierrors.NewTimeoutError("the request timed out", 1)
			}
			return false, nil, nil
		})
	}

	t.Run("another writer creates the first Secret during the move", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b")
		racing := true
		f.client.PrependReactor("create", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
			secret := action.(k8stesting.CreateAction).GetObject().(*corev1.Secret)
			if !racing || secret.Name != stateSecretName(current, 0) {
				return false, nil, nil
			}
			racing = false
			other := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secret.Name, Namespace: RadiusNamespace, Labels: stateLabels(current)},
				Data:       map[string][]byte{tfstateDataKey: []byte("other")},
			}
			require.NoError(t, f.client.Tracker().Create(corev1.SchemeGroupVersion.WithResource("secrets"), other, RadiusNamespace))
			return true, nil, apierrors.NewAlreadyExists(schema.GroupResource{Resource: "secrets"}, secret.Name)
		})

		// The lock stays held, so the step must not finish with the state locked.
		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		f.requireHeld(legacy)

		// The next pass finds the other state under the current name and releases the lock.
		require.Equal(t, hashmigration.Result{Conflicts: 1}, f.migrate())
		require.Equal(t, []string{"a", "b"}, f.state(legacy))
		f.requireFree(legacy)
	})

	t.Run("releasing the lock fails", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a")
		_, err := f.client.CoreV1().Secrets(RadiusNamespace).Create(t.Context(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "unexpected", Namespace: RadiusNamespace, Labels: stateLabels(legacy)},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
		failing := true
		f.client.PrependReactor("update", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if failing && action.(k8stesting.UpdateAction).GetObject().(*coordinationv1.Lease).Spec.HolderIdentity == nil {
				return true, nil, errors.New("connection reset")
			}
			return false, nil, nil
		})

		// Terraform ignores the lock duration, so the step must retry the release instead of finishing.
		require.Equal(t, hashmigration.Result{Skipped: 1, Remaining: 1}, f.migrate())
		f.requireHeld(legacy)

		failing = false
		require.Equal(t, hashmigration.Result{Skipped: 1}, f.migrate())
		require.Empty(t, f.chunkNames(current))
		f.requireFree(legacy)
	})

	t.Run("state cannot be read after an uncertain copy", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b", "c")
		failing := true
		failFirstCreate(f, current, &failing)
		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		f.requireHeld(legacy)

		// The copy may still become published, so a pass that cannot read the state keeps the lock and the copy.
		failing = false
		listing := true
		f.client.PrependReactor("list", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if listing && strings.Contains(action.(k8stesting.ListAction).GetListRestrictions().Labels.String(), legacy) {
				return true, nil, apierrors.NewServiceUnavailable("the API server is unavailable")
			}
			return false, nil, nil
		})
		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		f.requireHeld(legacy)
		require.Equal(t, map[int]string{1: stateSecretName(current, 1), 2: stateSecretName(current, 2)}, f.chunkNames(current))

		listing = false
		require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
		require.Equal(t, []string{"a", "b", "c"}, f.state(current))
		f.requireTombstone(legacy, current)
	})

	t.Run("operation waits for a lock kept after an uncertain copy", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b")
		failing := true
		failFirstCreate(f, current, &failing)
		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		f.requireHeld(legacy)

		// An operation resolved the legacy name and waits for the lock. While the earlier create may still be applied,
		// the copy is kept and the lock stays held.
		failing = false
		f.addResourceInState("redis", migrateApplicationID, "Updating")
		require.Equal(t, 1, f.migrate().Remaining)
		f.requireHeld(legacy)
		require.Equal(t, map[int]string{1: stateSecretName(current, 1)}, f.chunkNames(current))

		// Once the create can no longer be applied, the copy is removed and the operation gets the lock.
		f.now = f.now.Add(stateMigrationLeaseDuration + time.Second)
		require.Equal(t, 1, f.migrate().Remaining)
		require.Empty(t, f.chunkNames(current))
		require.Equal(t, []string{"a", "b"}, f.state(legacy))
		f.requireFree(legacy)
	})

	t.Run("operation waits for a lock kept after a failed commit", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b")
		failing := true
		f.client.PrependReactor("update", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if failing && action.(k8stesting.UpdateAction).GetObject().(*coordinationv1.Lease).Annotations[AnnotationStateMigratedParts] != "" {
				return true, nil, errors.New("connection reset")
			}
			return false, nil, nil
		})
		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		f.requireHeld(legacy)

		// The copy is published, so the move is committed although an operation is in progress. The operation fails
		// with the tombstone's message and its retry uses the moved state.
		failing = false
		f.addResourceInState("redis", migrateApplicationID, "Deleting")
		require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
		require.Empty(t, f.state(legacy))
		f.requireTombstone(legacy, current)
	})

	t.Run("owner deleted while the lock is kept", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b")
		failing := true
		f.client.PrependReactor("update", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if failing && action.(k8stesting.UpdateAction).GetObject().(*coordinationv1.Lease).Annotations[AnnotationStateMigratedParts] != "" {
				return true, nil, errors.New("connection reset")
			}
			return false, nil, nil
		})
		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		f.requireHeld(legacy)

		// The resource is deleted: its destroy uses the published copy and deletes its first Secret.
		failing = false
		require.NoError(t, f.client.CoreV1().Secrets(RadiusNamespace).Delete(t.Context(), stateSecretName(current, 0), metav1.DeleteOptions{}))
		require.NoError(t, f.db.Delete(t.Context(), migrateResourcePath+"redis"))

		// The kept lock would block a resource created again with the same name, so the step must not finish before
		// a pass releases it.
		require.Equal(t, 1, f.migrate().Remaining)
		f.requireFree(legacy)
		require.Empty(t, f.chunkNames(current))
		require.Equal(t, []string{"a", "b"}, f.state(legacy))
		require.Equal(t, hashmigration.Result{}, f.migrate())
	})

	t.Run("busy owner without a kept lock", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		_, legacy := f.addResourceInState("redis", migrateApplicationID, "Updating")
		f.addState(legacy, "a")
		f.setLease(legacy, new(StateMigrationHolderPrefix+"other"), f.now)

		require.Equal(t, hashmigration.Result{Remaining: 1}, f.migrate())
		require.Equal(t, StateMigrationHolderPrefix+"other", *f.lease(legacy).Spec.HolderIdentity)
	})
}

func Test_MigrateLegacyStates_UnfinishedMoves(t *testing.T) {
	t.Parallel()

	// keepAfterFailedCommit runs a pass whose commit fails, so the migrator keeps the lock of a published copy.
	keepAfterFailedCommit := func(f *stateFixture, legacy string) {
		failing := true
		f.client.PrependReactor("update", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if failing && action.(k8stesting.UpdateAction).GetObject().(*coordinationv1.Lease).Annotations[AnnotationStateMigratedParts] != "" {
				return true, nil, errors.New("connection reset")
			}
			return false, nil, nil
		})
		require.Equal(f.t, hashmigration.Result{Remaining: 1}, f.migrate())
		f.requireHeld(legacy)
		failing = false
	}

	// restart returns a migrator in a new process, after the lock of the stopped one expired.
	restart := func(f *stateFixture) {
		f.migrator = NewStateMigrator(f.client)
		f.migrator.now = func() time.Time { return f.now }
		f.now = f.now.Add(stateMigrationLeaseDuration + time.Second)
	}

	t.Run("restarted migrator releases the lock of a deleted resource", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b")
		keepAfterFailedCommit(f, legacy)
		require.Equal(t, current, f.lease(legacy).Annotations[AnnotationStateMigratedTo], "the lock records where the state moves")

		// The resource is deleted: its destroy uses the published copy and deletes its first Secret. Then the component
		// restarts, so the new migrator knows nothing of the lock it kept.
		require.NoError(t, f.client.CoreV1().Secrets(RadiusNamespace).Delete(t.Context(), stateSecretName(current, 0), metav1.DeleteOptions{}))
		require.NoError(t, f.db.Delete(t.Context(), migrateResourcePath+"redis"))
		restart(f)

		require.Equal(t, 1, f.migrate().Remaining)
		f.requireFree(legacy)
		require.NotContains(t, f.lease(legacy).Annotations, AnnotationStateMigratedTo, "a released lock does not record a move")
		require.Empty(t, f.chunkNames(current))
		require.Equal(t, []string{"a", "b"}, f.state(legacy))
		require.Equal(t, hashmigration.Result{}, f.migrate())
	})

	t.Run("restarted migrator commits a published copy of a deleted resource", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b")
		keepAfterFailedCommit(f, legacy)
		require.NoError(t, f.db.Delete(t.Context(), migrateResourcePath+"redis"))
		restart(f)

		require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
		require.Empty(t, f.state(legacy))
		require.Equal(t, []string{"a", "b"}, f.state(current))
		f.requireTombstone(legacy, current)
		require.Equal(t, current, f.lease(legacy).Annotations[AnnotationStateMigratedTo], "a tombstone records where the state moved")
	})

	t.Run("restarted migrator resolves a kept lock that an operation waits for", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a", "b")
		keepAfterFailedCommit(f, legacy)
		f.addResourceInState("redis", migrateApplicationID, "Updating")
		restart(f)

		require.Equal(t, hashmigration.Result{Migrated: 1}, f.migrate())
		f.requireTombstone(legacy, current)
	})

	t.Run("live migrator keeps its lock until it expires", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		current, legacy := f.addResource("redis", migrateApplicationID)
		f.addState(legacy, "a")
		require.NoError(t, f.db.Delete(t.Context(), migrateResourcePath+"redis"))
		f.setMigrationLease(legacy, current, new(StateMigrationHolderPrefix+"other"), f.now)

		require.Equal(t, 1, f.migrate().Remaining)
		require.Equal(t, StateMigrationHolderPrefix+"other", *f.lease(legacy).Spec.HolderIdentity)

		f.now = f.now.Add(stateMigrationLeaseDuration + time.Second)
		require.Equal(t, 1, f.migrate().Remaining, "the copy of a state whose resource is not known is not published")
		f.requireFree(legacy)
		require.Equal(t, hashmigration.Result{}, f.migrate())
	})

	t.Run("locks the migration does not hold are left alone", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		_, terraform := f.addResource("terraform", migrateApplicationID)
		f.setLease(terraform, new("terraform-run"), f.now.Add(-time.Hour))
		moved, tombstone := f.addResource("moved", migrateApplicationID)
		f.setMigrationLease(tombstone, moved, new(StateMigrationTombstone), f.now.Add(-time.Hour))
		_, free := f.addResource("free", migrateApplicationID)
		f.setLease(free, nil, f.now)

		require.Equal(t, hashmigration.Result{}, f.migrate())
		require.Equal(t, "terraform-run", *f.lease(terraform).Spec.HolderIdentity)
		require.Equal(t, StateMigrationTombstone, *f.lease(tombstone).Spec.HolderIdentity)
		f.requireFree(free)
	})

	t.Run("lock without a destination is reported", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		_, legacy := f.addResource("redis", migrateApplicationID)
		require.NoError(t, f.db.Delete(t.Context(), migrateResourcePath+"redis"))
		f.setLease(legacy, new(StateMigrationHolderPrefix+"other"), f.now.Add(-time.Hour))

		result, err := f.migrator.MigrateLegacyStates(t.Context(), f.db)
		require.NoError(t, err)
		require.Equal(t, 1, result.Skipped)
		require.Contains(t, result.Message, "does not record where the state moves")
	})

	t.Run("listing the locks fails", func(t *testing.T) {
		t.Parallel()
		f := newStateFixture(t)
		f.client.PrependReactor("list", "leases", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "coordination.k8s.io", Resource: "leases"}, "", errors.New("denied"))
		})

		_, err := f.migrator.MigrateLegacyStates(t.Context(), f.db)
		require.ErrorContains(t, err, "failed to list Terraform state locks")
	})
}

func Test_StateChunkIndex(t *testing.T) {
	t.Parallel()

	for name, expected := range map[string]int{
		"tfstate-default-abc":         0,
		"tfstate-default-abc-part-1":  1,
		"tfstate-default-abc-part-12": 12,
	} {
		index, ok := stateChunkIndex(name, "abc")
		require.True(t, ok, name)
		require.Equal(t, expected, index, name)
	}

	for _, name := range []string{"tfstate-default-abcd", "tfstate-default-abc-part-0", "tfstate-default-abc-part-01", "tfstate-default-abc-part-x", "other"} {
		_, ok := stateChunkIndex(name, "abc")
		require.False(t, ok, name)
	}
}
