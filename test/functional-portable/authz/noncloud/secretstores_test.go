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

package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	core "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/secretstore"
)

// CI retains synthetic records from dryRun across the enforce rollout, then exercises
// off and cleans up. An enforce invocation without those persisted fixtures must fail.
func Test_SecretStores_ProtectedReferences(t *testing.T) {
	options := rp.NewRPTestOptions(t)
	client := secretstore.New(t, options)
	mode := expectedInstalledMode(t)
	id := os.Getenv("SECRETSTORE_AUTHZ_ID")
	if mode == authz.ModeEnforce {
		require.NotEmpty(t, id, "enforce requires SECRETSTORE_AUTHZ_ID from a prior off/dryRun run with SECRETSTORE_AUTHZ_RETAIN=true")
	}
	if id == "" {
		id = fmt.Sprintf("ss-authz-%d", time.Now().UnixNano())
	}
	retain := os.Getenv("SECRETSTORE_AUTHZ_RETAIN") == "true"
	const ownerLabel = "radius-test-secretstore"
	const initial = "c3ludGhldGljLWluaXRpYWw="
	const updated = "c3ludGhldGljLXVwZGF0ZWQ="
	ctx := t.Context()
	secrets := options.K8sClient.CoreV1().Secrets(radiusNamespace)

	for _, tt := range []struct{ suffix, secret, rule string }{
		{"named", "target-kubeconfig", authz.RuleControlPlaneSecret},
		{"namespace", id, authz.RuleControlPlaneNamespace},
	} {
		t.Run(tt.suffix, func(t *testing.T) {
			name := id + "-" + tt.suffix
			ref := radiusNamespace + "/" + tt.secret
			target := "/" + client.Scope + "/providers/Applications.Core/secretStores/" + name
			start := metav1.Now()
			backing, err := secrets.Get(ctx, tt.secret, metav1.GetOptions{})
			switch {
			case apierrors.IsNotFound(err):
				require.NotEqual(t, authz.ModeEnforce, mode, "enforce fixture is missing")
				requireSecretUnused(t, options, tt.secret)
				backing, err = secrets.Create(ctx, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: tt.secret, Labels: map[string]string{ownerLabel: id}},
					Data:       map[string][]byte{"token": []byte(initial)},
				}, metav1.CreateOptions{})
				require.NoError(t, err)
			default:
				require.NoError(t, err)
				require.Equal(t, id, backing.Labels[ownerLabel], "refusing to read or mutate a Secret not created by this test run")
				requireSecretUnused(t, options, tt.secret)
			}
			uid := backing.UID
			if mode != authz.ModeEnforce && !retain {
				t.Cleanup(func() {
					cleanup := context.WithoutCancel(ctx)
					assert.NoError(t, client.Delete(cleanup, name))
					err := secrets.Delete(cleanup, tt.secret, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
					assert.True(t, err == nil || apierrors.IsNotFound(err), "fixture cleanup: %v", err)
				})
			}

			deny := func(err error) {
				t.Helper()
				var response *azcore.ResponseError
				require.ErrorAs(t, err, &response)
				require.Equal(t, http.StatusForbidden, response.StatusCode)
				require.Equal(t, v1.CodeAdmissionPolicyDenied, response.ErrorCode)
				require.Contains(t, response.Error(), "admission rule '"+tt.rule+"'")
			}
			if mode == authz.ModeEnforce {
				record, err := client.API.Get(ctx, client.Scope, name, nil)
				require.NoError(t, err, "existing record must have been persisted before enabling enforcement")
				deny(client.Put(ctx, name+"-new", secretstore.Resource(ref, initial)))
				client.RequireGone(t, name+"-new")
				deny(client.Put(ctx, name, secretstore.Resource(ref, initial)))
				deny(client.Patch(ctx, name, secretstore.Resource("", updated)))
				result, err := client.API.ListSecrets(ctx, client.Scope, name, core.ListSecretsRequest{}, nil)
				deny(err)
				require.Empty(t, result.Data, "denied listSecrets must return no values")
				deny(client.Delete(ctx, name))
				after, err := client.API.Get(ctx, client.Scope, name, nil)
				require.NoError(t, err)
				require.Equal(t, record.SecretStoreResource, after.SecretStoreResource, "denied requests must not change the record")
				requireSecretStoreDecisions(t, options, start, target+"-new", tt.rule, mode, map[string]int{"write": 1})
			} else {
				require.NoError(t, client.Put(ctx, name, secretstore.Resource(ref, initial)))
				client.RequireValue(t, name, ref, initial)
				require.NoError(t, client.Patch(ctx, name, secretstore.Resource("", updated)))
				client.RequireValue(t, name, ref, updated)
				beforeDelete, err := secrets.Get(ctx, tt.secret, metav1.GetOptions{})
				require.NoError(t, err)
				require.NoError(t, client.Delete(ctx, name))
				client.RequireGone(t, name)
				afterDelete, err := secrets.Get(ctx, tt.secret, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, beforeDelete.ResourceVersion, afterDelete.ResourceVersion, "adopted Secret must not be changed by DELETE")
				require.NoError(t, client.Put(ctx, name, secretstore.Resource(ref, "")))
				client.RequireValue(t, name, ref, updated)
			}
			after, err := secrets.Get(ctx, tt.secret, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, uid, after.UID)
			if mode == authz.ModeEnforce {
				require.Equal(t, backing.ResourceVersion, after.ResourceVersion, "denied operations must not mutate backing data or metadata")
			}
			expected := map[string]int{"write": 3, "listSecrets/action": 3, "delete": 1}
			if mode == authz.ModeEnforce {
				expected = map[string]int{"write": 2, "listSecrets/action": 1, "delete": 1}
			}
			requireSecretStoreDecisions(t, options, start, target, tt.rule, mode, expected)
		})
	}
}

// Never use a real control-plane credential as a test fixture, even when its name is
// protected. The default target-kubeconfig is unused on this single-cluster CI leg.
func requireSecretUnused(t *testing.T, options rp.RPTestOptions, name string) {
	t.Helper()
	pods, err := options.K8sClient.CoreV1().Pods(radiusNamespace).List(t.Context(), metav1.ListOptions{})
	require.NoError(t, err)
	for _, pod := range pods.Items {
		for _, volume := range pod.Spec.Volumes {
			if volume.Secret != nil {
				require.NotEqual(t, name, volume.Secret.SecretName, "fixture is mounted by %s", pod.Name)
			}
			if volume.Projected != nil {
				for _, source := range volume.Projected.Sources {
					if source.Secret != nil {
						require.NotEqual(t, name, source.Secret.Name, "fixture is projected by %s", pod.Name)
					}
				}
			}
		}
		containers := append(append([]corev1.Container{}, pod.Spec.InitContainers...), pod.Spec.Containers...)
		for _, container := range containers {
			for _, env := range container.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					require.NotEqual(t, name, env.ValueFrom.SecretKeyRef.Name)
				}
			}
			for _, env := range container.EnvFrom {
				if env.SecretRef != nil {
					require.NotEqual(t, name, env.SecretRef.Name)
				}
			}
		}
		for _, secret := range pod.Spec.ImagePullSecrets {
			require.NotEqual(t, name, secret.Name)
		}
	}
}

func requireSecretStoreDecisions(t *testing.T, options rp.RPTestOptions, start metav1.Time, target, rule string, mode authz.Mode, expected map[string]int) {
	t.Helper()
	pods, err := options.K8sClient.CoreV1().Pods(radiusNamespace).List(t.Context(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=applications-rp",
	})
	require.NoError(t, err)
	require.NotEmpty(t, pods.Items)
	require.Eventually(t, func() bool {
		actions := map[string]int{}
		for _, pod := range pods.Items {
			logs, err := options.K8sClient.CoreV1().Pods(radiusNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{
				Container: "applications-rp", SinceTime: &start,
			}).DoRaw(t.Context())
			require.NoError(t, err)
			for _, line := range strings.Split(string(logs), "\n") {
				var entry map[string]any
				if json.Unmarshal([]byte(line), &entry) != nil || entry[authz.LogFieldTarget] != target {
					continue
				}
				if entry[authz.LogFieldWouldDeny] != true && entry[authz.LogFieldDenied] != true {
					continue
				}
				require.NotEqual(t, authz.ModeOff, mode, "off must not emit denial decisions")
				require.Equal(t, rule, entry[authz.LogFieldRule])
				require.Equal(t, v1.CodeAdmissionPolicyDenied, entry[authz.LogFieldCode])
				if mode == authz.ModeDryRun {
					require.Equal(t, true, entry[authz.LogFieldWouldDeny])
					require.NotEqual(t, true, entry[authz.LogFieldDenied])
				} else {
					require.Equal(t, true, entry[authz.LogFieldDenied])
					require.NotEqual(t, true, entry[authz.LogFieldWouldDeny])
				}
				action, ok := entry[authz.LogFieldAction].(string)
				require.True(t, ok)
				actions[action]++
			}
		}
		if mode == authz.ModeOff {
			return true
		}
		for action, count := range expected {
			if actions["Applications.Core/secretStores/"+action] < count {
				return false
			}
		}
		return true
	}, 30*time.Second, time.Second, "missing exact secretStore admission decisions for %s", target)
}
