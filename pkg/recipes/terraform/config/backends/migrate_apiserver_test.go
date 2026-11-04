//go:build !windows

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
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
)

// Test_StateMigrator_APIServer moves Terraform states on a real API server, which, unlike the fake clientset, enforces
// delete preconditions, resource versions, and conflicting writes.
func Test_StateMigrator_APIServer(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to move Terraform states on an isolated API server.")
	}

	// A regression must fail on this unusable config, never reach a user's cluster.
	unusedKubeconfig := filepath.Join(t.TempDir(), "unused-kubeconfig")
	require.NoError(t, os.WriteFile(unusedKubeconfig, []byte("invalid kubeconfig"), 0600))
	t.Setenv("KUBECONFIG", unusedKubeconfig)
	useExistingCluster := false
	environment := &envtest.Environment{UseExistingCluster: &useExistingCluster}
	config, err := environment.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, environment.Stop())
	})
	client, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)
	_, err = client.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: RadiusNamespace}}, metav1.CreateOptions{})
	require.NoError(t, err)

	db := inmemory.NewClient()
	addResource := func(t *testing.T, name string) (string, string) {
		id := migrateResourcePath + name
		data := map[string]any{"id": id, "provisioningState": "Succeeded", "properties": map[string]any{"environment": migrateEnvironmentID, "application": migrateApplicationID}}
		require.NoError(t, db.Save(t.Context(), &database.Object{ID: id, Data: data}))
		current, legacy, err := SecretSuffixes(&recipes.ResourceMetadata{ResourceID: id, EnvironmentID: migrateEnvironmentID, ApplicationID: migrateApplicationID})
		require.NoError(t, err)
		return current, legacy
	}

	addState := func(t *testing.T, suffix string, parts ...string) []corev1.Secret {
		secrets := []corev1.Secret{}
		for index, part := range parts {
			secret, err := client.CoreV1().Secrets(RadiusNamespace).Create(t.Context(), &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: stateSecretName(suffix, index), Labels: stateLabels(suffix), Annotations: map[string]string{"encoding": "gzip"}},
				Data:       map[string][]byte{tfstateDataKey: []byte(part)},
			}, metav1.CreateOptions{})
			require.NoError(t, err)
			secrets = append(secrets, *secret)
		}
		return secrets
	}

	state := func(t *testing.T, migrator *StateMigrator, suffix string) []string {
		chunks, err := migrator.listChunks(t.Context(), suffix)
		require.NoError(t, err)
		ordered, ok := orderedChunks(chunks)
		require.True(t, ok)
		parts := []string{}
		for _, chunk := range ordered {
			parts = append(parts, string(chunk.Data[tfstateDataKey]))
		}
		return parts
	}

	requireTombstone := func(t *testing.T, legacy string, current string) {
		lease, err := client.CoordinationV1().Leases(RadiusNamespace).Get(t.Context(), stateLockName(legacy), metav1.GetOptions{})
		require.NoError(t, err)
		require.NotNil(t, lease.Spec.HolderIdentity, "Terraform must not be able to acquire the lock")
		require.Equal(t, StateMigrationTombstone, *lease.Spec.HolderIdentity)
		require.Equal(t, "tombstone", lease.Labels[LabelStateMigration])

		// Terraform decodes this annotation into its LockInfo to report who holds the lock.
		info := struct {
			ID        string
			Operation string
			Info      string
			Who       string
			Version   string
			Created   time.Time
			Path      string
		}{}
		require.NoError(t, json.Unmarshal([]byte(lease.Annotations[tfstateLockInfoAnnotation]), &info))
		require.Equal(t, StateMigrationTombstone, info.ID)
		require.Contains(t, info.Info, stateSecretName(current, 0))
		require.False(t, info.Created.IsZero())
	}

	t.Run("moves a state in parts", func(t *testing.T) {
		current, legacy := addResource(t, "parts")
		sources := addState(t, legacy, "a", "b", "c")
		migrator := NewStateMigrator(client)

		result, err := migrator.MigrateLegacyStates(t.Context(), db)
		require.NoError(t, err)
		require.Equal(t, 1, result.Migrated)
		require.Equal(t, []string{"a", "b", "c"}, state(t, migrator, current))
		require.Empty(t, state(t, migrator, legacy))
		requireTombstone(t, legacy, current)

		for index, source := range sources {
			copied, err := client.CoreV1().Secrets(RadiusNamespace).Get(t.Context(), stateSecretName(current, index), metav1.GetOptions{})
			require.NoError(t, err)
			require.NotEqual(t, source.UID, copied.UID)
			require.Equal(t, string(source.UID), copied.Annotations[AnnotationStateMigratedFromUID])
			require.Equal(t, source.ResourceVersion, copied.Annotations[AnnotationStateMigratedFromVersion])
			require.Equal(t, current, copied.Labels[tfstateSecretSuffixLabel])
		}

		// Another migrator, such as one in another component, finds nothing left to move.
		result, err = NewStateMigrator(client).MigrateLegacyStates(t.Context(), db)
		require.NoError(t, err)
		require.Zero(t, result.Migrated+result.Remaining+result.Conflicts+result.Skipped)
		requireTombstone(t, legacy, current)
	})

	t.Run("waits for a Terraform lock", func(t *testing.T) {
		current, legacy := addResource(t, "locked")
		addState(t, legacy, "state")

		// Lock the state the way Terraform does.
		holder := "terraform-run"
		lease, err := client.CoordinationV1().Leases(RadiusNamespace).Create(t.Context(), &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Name: stateLockName(legacy), Labels: stateLabels(legacy), Annotations: map[string]string{tfstateLockInfoAnnotation: `{"ID":"terraform-run"}`}},
			Spec:       coordinationv1.LeaseSpec{HolderIdentity: &holder},
		}, metav1.CreateOptions{})
		require.NoError(t, err)

		migrator := NewStateMigrator(client)
		result, err := migrator.MigrateLegacyStates(t.Context(), db)
		require.NoError(t, err)
		require.Equal(t, 1, result.Remaining)
		require.Empty(t, state(t, migrator, current))

		// Unlock the state the way Terraform does; the next pass moves it.
		lease.Spec.HolderIdentity = nil
		delete(lease.Annotations, tfstateLockInfoAnnotation)
		_, err = client.CoordinationV1().Leases(RadiusNamespace).Update(t.Context(), lease, metav1.UpdateOptions{})
		require.NoError(t, err)

		result, err = migrator.MigrateLegacyStates(t.Context(), db)
		require.NoError(t, err)
		require.Equal(t, 1, result.Migrated)
		require.Equal(t, []string{"state"}, state(t, migrator, current))
		requireTombstone(t, legacy, current)
	})

	t.Run("does not delete a legacy state that changed", func(t *testing.T) {
		_, legacy := addResource(t, "changed")
		sources := addState(t, legacy, "state")
		changed := sources[0].DeepCopy()
		changed.Data[tfstateDataKey] = []byte("changed")
		_, err := client.CoreV1().Secrets(RadiusNamespace).Update(t.Context(), changed, metav1.UpdateOptions{})
		require.NoError(t, err)

		err = NewStateMigrator(client).deleteChunks(t.Context(), sources)
		require.True(t, apierrors.IsConflict(err), "a stale resource version must fail the delete: %v", err)
		_, err = client.CoreV1().Secrets(RadiusNamespace).Get(t.Context(), sources[0].Name, metav1.GetOptions{})
		require.NoError(t, err)
	})

	t.Run("concurrent migrators move a state once", func(t *testing.T) {
		current, legacy := addResource(t, "concurrent")
		addState(t, legacy, "a", "b")

		results := make([]hashmigration.Result, 4)
		var wait sync.WaitGroup
		for index := range results {
			wait.Add(1)
			go func() {
				defer wait.Done()
				results[index] = NewStateMigrator(client).MoveState(t.Context(), legacy, current)
			}()
		}
		wait.Wait()

		total := hashmigration.Result{}
		for _, result := range results {
			total.Add(result)
		}
		require.Equal(t, 1, total.Migrated, "%+v", results)
		require.Zero(t, total.Conflicts+total.Skipped, "%+v", results)

		migrator := NewStateMigrator(client)
		require.Equal(t, []string{"a", "b"}, state(t, migrator, current))
		require.Empty(t, state(t, migrator, legacy))
		requireTombstone(t, legacy, current)
	})
}
