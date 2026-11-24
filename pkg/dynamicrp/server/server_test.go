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

package server

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/dynamicrp"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
)

func Test_NewServer_HashMigration(t *testing.T) {
	t.Parallel()
	for _, disabled := range []bool{false, true} {
		options := &dynamicrp.Options{
			Config:             &dynamicrp.Config{SHA256Migration: hashmigration.Config{Disabled: disabled}},
			DatabaseProvider:   databaseprovider.FromMemory(),
			KubernetesProvider: kubernetesclientprovider.FromConfig(nil),
		}
		host, err := NewServer(options)
		require.NoError(t, err)

		found := false
		for _, service := range host.Services {
			if _, ok := service.(*hashmigration.Service); ok {
				found = true
			}
		}
		require.Equal(t, !disabled, found)
	}
}

func Test_HashMigrationService(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	environment := "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/environments/env"
	resource := "/planes/radius/local/resourceGroups/test-group/providers/Test.Resources/userTypes/resource"

	databaseProvider := databaseprovider.FromMemory()
	db, err := databaseProvider.GetClient(ctx)
	require.NoError(t, err)
	data := map[string]any{"id": resource, "properties": map[string]any{"environment": environment}}
	require.NoError(t, db.Save(ctx, &database.Object{ID: resource, Data: data}))

	current, legacy, err := backends.SecretSuffixes(&recipes.ResourceMetadata{ResourceID: resource, EnvironmentID: environment})
	require.NoError(t, err)
	client := fake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backends.KubernetesBackendNamePrefix + legacy,
			Namespace: backends.RadiusNamespace,
			Labels:    map[string]string{"tfstate": "true", "tfstateSecretSuffix": legacy, "tfstateWorkspace": "default"},
		},
		Data: map[string][]byte{"tfstate": []byte("state")},
	})
	kubernetesProvider := kubernetesclientprovider.FromConfig(nil)
	kubernetesProvider.SetClientGoClient(client)

	options := &dynamicrp.Options{DatabaseProvider: databaseProvider, KubernetesProvider: kubernetesProvider}
	require.True(t, newHashMigrationService(options).RunPass(ctx))

	statuses, err := hashmigration.ReadStatus(ctx, client, hashmigration.DefaultNamespace)
	require.NoError(t, err)
	status := statuses[hashmigration.StatusKey(hashMigrationComponent, backends.StateMigrationStepName)]
	require.Equal(t, hashmigration.StateComplete, status.State)
	require.Equal(t, 1, status.Migrated)

	_, err = client.CoreV1().Secrets(backends.RadiusNamespace).Get(ctx, backends.KubernetesBackendNamePrefix+current, metav1.GetOptions{})
	require.NoError(t, err)
}

func Test_HashMigrationService_WithoutKubernetes(t *testing.T) {
	t.Parallel()
	options := &dynamicrp.Options{DatabaseProvider: databaseprovider.FromMemory()}
	require.False(t, newHashMigrationService(options).RunPass(t.Context()))
}
