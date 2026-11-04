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
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	k8slabels "github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
)

func Test_HashMigrationService(t *testing.T) {
	ctx := t.Context()
	environment := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/env"
	resource := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/extenders/extender"

	databaseProvider := databaseprovider.FromMemory()
	db, err := databaseProvider.GetClient(ctx)
	require.NoError(t, err)
	data := map[string]any{"id": resource, "provisioningState": "Succeeded", "properties": map[string]any{"environment": environment}}
	require.NoError(t, db.Save(ctx, &database.Object{ID: resource, Data: data}))

	// A Terraform state and a container workload written by an older version of Radius.
	current, legacy, err := backends.SecretSuffixes(&recipes.ResourceMetadata{ResourceID: resource, EnvironmentID: environment})
	require.NoError(t, err)
	secretData := map[string][]byte{"KEY": []byte("value")}
	client := fake.NewClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      backends.KubernetesBackendNamePrefix + legacy,
				Namespace: backends.RadiusNamespace,
				Labels:    map[string]string{"tfstate": "true", "tfstateSecretSuffix": legacy, "tfstateWorkspace": "default"},
			},
			Data: map[string][]byte{"tfstate": []byte("state")},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "app", Labels: map[string]string{k8slabels.LabelRadiusResource: "frontend"}},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{k8slabels.AnnotationSecretHash: k8slabels.LegacyHashSecretData(secretData)},
			}}},
		},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "app"}, Data: secretData},
	)

	clientFn := func() (kubernetes.Interface, error) { return client, nil }
	targetFn := func(context.Context) (kubernetes.Interface, error) { return client, nil }
	reporter := hashmigration.NewConfigMapReporter(client, hashmigration.DefaultNamespace)
	require.True(t, newHashMigrationService(databaseProvider, clientFn, targetFn, reporter).RunPass(ctx))

	statuses, err := hashmigration.ReadStatus(ctx, client, hashmigration.DefaultNamespace)
	require.NoError(t, err)
	for _, step := range []string{backends.StateMigrationStepName, "containerSecretHash"} {
		status := statuses[hashmigration.StatusKey(HashMigrationComponent, step)]
		require.Equal(t, hashmigration.StateComplete, status.State, step)
		require.Equal(t, 1, status.Migrated, step)
	}

	_, err = client.CoreV1().Secrets(backends.RadiusNamespace).Get(ctx, backends.KubernetesBackendNamePrefix+current, metav1.GetOptions{})
	require.NoError(t, err)
	deployment, err := client.AppsV1().Deployments("app").Get(ctx, "frontend", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, k8slabels.HashSecretData(secretData), deployment.Annotations[k8slabels.AnnotationSecretHashSHA256])
}

func Test_NewHashMigrationService_WithoutKubernetes(t *testing.T) {
	options := hostoptions.HostOptions{Config: &hostoptions.ProviderConfig{
		DatabaseProvider: databaseprovider.Options{Provider: databaseprovider.TypeInMemory},
	}}

	// Without a cluster both steps fail and are retried; nothing panics.
	require.False(t, NewHashMigrationService(options).RunPass(t.Context()))
}
