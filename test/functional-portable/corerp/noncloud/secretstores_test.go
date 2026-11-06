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

package corerp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/radius-project/radius/pkg/azure/tokencredentials"
	core "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/secretstore"
)

func Test_SecretStores_Lifecycle(t *testing.T) {
	options := rp.NewRPTestOptions(t)
	client := secretstore.New(t, options)
	ctx := t.Context()
	namespace := fmt.Sprintf("ss-%d", time.Now().UnixNano())
	_, err := options.K8sClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		err := options.K8sClient.CoreV1().Namespaces().Delete(context.WithoutCancel(ctx), namespace, metav1.DeleteOptions{})
		assert.NoError(t, err)
	})
	secrets := options.K8sClient.CoreV1().Secrets(namespace)
	const initial = "c3ludGhldGljLWluaXRpYWw="
	const updated = "c3ludGhldGljLXVwZGF0ZWQ="

	for _, scenario := range []string{"owned", "adopted", "labeled", "replacement", "missing", "shared", "lookalike"} {
		t.Run(scenario, func(t *testing.T) {
			name := namespace + "-" + scenario
			backingName := scenario
			if scenario == "lookalike" {
				backingName = "database-secret"
			}
			ref := namespace + "/" + backingName
			if scenario == "adopted" || scenario == "labeled" {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: backingName},
					Data:       map[string][]byte{"token": []byte(initial)},
				}
				if scenario == "labeled" {
					secret.Labels = kubernetes.MakeDescriptiveLabels("", backingName, "Applications.Core/secretStores")
				}
				_, err := secrets.Create(ctx, secret, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			t.Cleanup(func() { assert.NoError(t, client.Delete(context.WithoutCancel(ctx), name)) })
			require.NoError(t, client.Put(ctx, name, secretstore.Resource(ref, initial)))
			client.RequireValue(t, name, ref, initial)
			original, err := secrets.Get(ctx, backingName, metav1.GetOptions{})
			require.NoError(t, err)
			require.NotEmpty(t, original.UID)

			// Omit the reference on PATCH to exercise restoration from the persisted record.
			require.NoError(t, client.Patch(ctx, name, secretstore.Resource("", updated)))
			client.RequireValue(t, name, ref, updated)
			beforeDelete, err := secrets.Get(ctx, backingName, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, original.UID, beforeDelete.UID)
			require.True(t, string(beforeDelete.Data["token"]) == updated, "PATCH did not update backing data")

			if scenario == "shared" {
				adopter := name + "-adopter"
				t.Cleanup(func() { assert.NoError(t, client.Delete(context.WithoutCancel(ctx), adopter)) })
				require.NoError(t, client.Put(ctx, adopter, secretstore.Resource(ref, "")))
				require.NoError(t, client.Delete(ctx, adopter))
				client.RequireGone(t, adopter)
				client.RequireValue(t, name, ref, updated)
				retained, err := secrets.Get(ctx, backingName, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, beforeDelete.UID, retained.UID)
			}
			if scenario == "replacement" || scenario == "missing" {
				require.NoError(t, secrets.Delete(ctx, backingName, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &original.UID}}))
				if scenario == "replacement" {
					beforeDelete, err = secrets.Create(ctx, &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: backingName, Labels: original.Labels},
						Data:       map[string][]byte{"token": []byte(initial)},
					}, metav1.CreateOptions{})
					require.NoError(t, err)
					require.NotEqual(t, original.UID, beforeDelete.UID)
				}
			}

			require.NoError(t, client.Delete(ctx, name))
			client.RequireGone(t, name)
			require.NoError(t, client.Delete(ctx, name), "repeat DELETE must be idempotent")
			retained, err := secrets.Get(ctx, backingName, metav1.GetOptions{})
			keep := scenario == "adopted" || scenario == "labeled" || scenario == "replacement"
			if keep {
				require.NoError(t, err)
				require.Equal(t, beforeDelete.UID, retained.UID)
				require.Equal(t, beforeDelete.ResourceVersion, retained.ResourceVersion, "DELETE must not mutate retained Secrets")
			} else {
				require.True(t, apierrors.IsNotFound(err), "owned or missing Secret must be absent: %v", err)
			}

			require.NoError(t, client.Put(ctx, name, secretstore.Resource(ref, initial)))
			client.RequireValue(t, name, ref, initial)
			recreated, err := secrets.Get(ctx, backingName, metav1.GetOptions{})
			require.NoError(t, err)
			if keep {
				require.Equal(t, beforeDelete.UID, recreated.UID)
			} else {
				require.NotEqual(t, original.UID, recreated.UID)
			}
			require.NoError(t, client.Delete(ctx, name))
			client.RequireGone(t, name)
			retained, err = secrets.Get(ctx, backingName, metav1.GetOptions{})
			if keep {
				require.NoError(t, err)
				require.Equal(t, recreated.UID, retained.UID)
			} else {
				require.True(t, apierrors.IsNotFound(err))
			}
		})
	}

	t.Run("derived namespace", func(t *testing.T) {
		opts := sdk.NewClientOptions(options.Connection)
		environments, err := core.NewEnvironmentsClient(&tokencredentials.AnonymousCredential{}, opts)
		require.NoError(t, err)
		applications, err := core.NewApplicationsClient(&tokencredentials.AnonymousCredential{}, opts)
		require.NoError(t, err)
		env, err := environments.CreateOrUpdate(ctx, client.Scope, namespace, core.EnvironmentResource{
			Location: new("global"),
			Properties: &core.EnvironmentProperties{Compute: &core.KubernetesCompute{
				Kind: new("kubernetes"), Namespace: new(namespace),
			}},
		}, nil)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, err := environments.Delete(context.WithoutCancel(ctx), client.Scope, namespace, nil)
			assert.NoError(t, err)
		})
		for _, scope := range []string{"inherited", "override"} {
			t.Run(scope, func(t *testing.T) {
				name := namespace + "-" + scope
				appProperties := &core.ApplicationProperties{Environment: env.ID}
				targetNamespace := namespace + "-" + name
				if scope == "override" {
					targetNamespace = namespace + "-custom"
					appProperties.Extensions = []core.ExtensionClassification{
						&core.KubernetesNamespaceExtension{Kind: new("kubernetesNamespace"), Namespace: new(targetNamespace)},
					}
				}
				app, err := applications.CreateOrUpdate(ctx, client.Scope, name, core.ApplicationResource{
					Location: new("global"), Properties: appProperties,
				}, nil)
				require.NoError(t, err)
				t.Cleanup(func() {
					cleanup := context.WithoutCancel(ctx)
					_, err := applications.Delete(cleanup, client.Scope, name, nil)
					assert.NoError(t, err)
					err = options.K8sClient.CoreV1().Namespaces().Delete(cleanup, targetNamespace, metav1.DeleteOptions{})
					assert.True(t, err == nil || apierrors.IsNotFound(err), "namespace cleanup: %v", err)
				})
				secrets := options.K8sClient.CoreV1().Secrets(targetNamespace)
				body := secretstore.Resource("", initial)
				body.Properties.Application = app.ID
				t.Cleanup(func() { assert.NoError(t, client.Delete(context.WithoutCancel(ctx), name)) })
				require.NoError(t, client.Put(ctx, name, body))
				client.RequireValue(t, name, targetNamespace+"/"+name, initial)
				before, err := secrets.Get(ctx, name, metav1.GetOptions{})
				require.NoError(t, err)
				body.Properties.Data["token"].Value = new(updated)
				require.NoError(t, client.Patch(ctx, name, body))
				client.RequireValue(t, name, targetNamespace+"/"+name, updated)
				after, err := secrets.Get(ctx, name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, before.UID, after.UID)
				require.NoError(t, client.Delete(ctx, name))
				client.RequireGone(t, name)
				_, err = secrets.Get(ctx, name, metav1.GetOptions{})
				require.True(t, apierrors.IsNotFound(err))
			})
		}
	})
}
