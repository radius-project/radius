/*
Copyright 2023 The Radius Authors.

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

package tfstate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func tfstateSecret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       DefaultNamespace,
			Labels:          map[string]string{"tfstate": "true"},
			ResourceVersion: "12345",
			UID:             "abcde-uid",
		},
		Data: data,
	}
}

func Test_Backup_WritesLabelledSecretsOnly(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		tfstateSecret("tfstate-default-aaa", map[string][]byte{"tfstate": []byte("state-a")}),
		tfstateSecret("tfstate-default-bbb", map[string][]byte{"tfstate": []byte("state-b")}),
		// A secret without the tfstate label must be ignored.
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "database-secret", Namespace: DefaultNamespace},
			Data:       map[string][]byte{"POSTGRES_PASSWORD": []byte("nope")},
		},
	)

	stateDir := t.TempDir()
	client := NewClient(clientset, DefaultNamespace)

	err := client.Backup(context.Background(), stateDir)
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(stateDir, SubDir))
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.FileExists(t, filepath.Join(stateDir, SubDir, "tfstate-default-aaa.json"))
	require.FileExists(t, filepath.Join(stateDir, SubDir, "tfstate-default-bbb.json"))
	require.NoFileExists(t, filepath.Join(stateDir, SubDir, "database-secret.json"))
}

func Test_Backup_ClearsStaleFiles(t *testing.T) {
	stateDir := t.TempDir()
	staleDir := filepath.Join(stateDir, SubDir)
	require.NoError(t, os.MkdirAll(staleDir, 0o755))
	stalePath := filepath.Join(staleDir, "tfstate-default-deleted.json")
	require.NoError(t, os.WriteFile(stalePath, []byte("{}"), 0o644))

	clientset := fake.NewSimpleClientset(
		tfstateSecret("tfstate-default-live", map[string][]byte{"tfstate": []byte("live")}),
	)
	client := NewClient(clientset, DefaultNamespace)

	err := client.Backup(context.Background(), stateDir)
	require.NoError(t, err)

	require.NoFileExists(t, stalePath)
	require.FileExists(t, filepath.Join(staleDir, "tfstate-default-live.json"))
}

func Test_Backup_NoSecrets(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	stateDir := t.TempDir()
	client := NewClient(clientset, DefaultNamespace)

	err := client.Backup(context.Background(), stateDir)
	require.NoError(t, err)
	require.False(t, HasBackup(stateDir))
}

func Test_HasBackup(t *testing.T) {
	stateDir := t.TempDir()
	require.False(t, HasBackup(stateDir))

	dir := filepath.Join(stateDir, SubDir)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.False(t, HasBackup(stateDir), "empty tfstate dir is not a backup")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "tfstate-default-aaa.json"), []byte("{}"), 0o644))
	require.True(t, HasBackup(stateDir))
}

func Test_BackupRestore_RoundTrip(t *testing.T) {
	original := tfstateSecret("tfstate-default-aaa", map[string][]byte{"tfstate": []byte("round-trip-state")})
	source := fake.NewSimpleClientset(original)
	stateDir := t.TempDir()

	require.NoError(t, NewClient(source, DefaultNamespace).Backup(context.Background(), stateDir))

	// Restore into a fresh, empty cluster.
	target := fake.NewSimpleClientset()
	require.NoError(t, NewClient(target, DefaultNamespace).Restore(context.Background(), stateDir))

	restored, err := target.CoreV1().Secrets(DefaultNamespace).Get(context.Background(), "tfstate-default-aaa", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, []byte("round-trip-state"), restored.Data["tfstate"])
	require.Equal(t, "true", restored.Labels["tfstate"])
	// Server-managed fields must not be carried over from the source cluster.
	require.Empty(t, restored.UID)
}

func Test_Restore_UpdatesExistingSecret(t *testing.T) {
	stateDir := t.TempDir()
	source := fake.NewSimpleClientset(
		tfstateSecret("tfstate-default-aaa", map[string][]byte{"tfstate": []byte("new-state")}),
	)
	require.NoError(t, NewClient(source, DefaultNamespace).Backup(context.Background(), stateDir))

	// Target already has a secret of the same name with stale data.
	target := fake.NewSimpleClientset(
		tfstateSecret("tfstate-default-aaa", map[string][]byte{"tfstate": []byte("old-state")}),
	)
	require.NoError(t, NewClient(target, DefaultNamespace).Restore(context.Background(), stateDir))

	updated, err := target.CoreV1().Secrets(DefaultNamespace).Get(context.Background(), "tfstate-default-aaa", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, []byte("new-state"), updated.Data["tfstate"])
}

func Test_Restore_NoBackupIsNoOp(t *testing.T) {
	target := fake.NewSimpleClientset()
	stateDir := t.TempDir()

	err := NewClient(target, DefaultNamespace).Restore(context.Background(), stateDir)
	require.NoError(t, err)

	secrets, err := target.CoreV1().Secrets(DefaultNamespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, secrets.Items)
}
