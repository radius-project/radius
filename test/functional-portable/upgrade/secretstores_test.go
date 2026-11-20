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

package upgrade_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/secretstore"
	"github.com/radius-project/radius/test/testutil"
)

// Test_SecretStores_Upgrade creates the record with released pre-ownership code, not
// the candidate binary or a version-renamed candidate chart. Only then is Radius upgraded.
func Test_SecretStores_Upgrade(t *testing.T) {
	const baseline = "0.62.0"
	const initial = "c3ludGhldGljLWxlZ2FjeQ=="
	const updated = "c3ludGhldGljLXVwZ3JhZGVk"
	ctx := t.Context()
	k8sClient, err := newKubernetesClient()
	require.NoError(t, err)
	cleanupAndWait(t, ctx, k8sClient)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupCommandTimeout)
		defer cancel()
		helmUninstall(t, cleanup)
	})
	baselineArgs := []string{
		"helm", "install", helmReleaseName, "oci://ghcr.io/radius-project/helm-chart/radius",
		"--version", baseline, "--namespace", radiusNamespace, "--create-namespace",
		"--wait", "--timeout", helmTimeout, "--set", "preupgrade.enabled=false",
	}
	for _, component := range radiusImageComponents {
		baselineArgs = append(baselineArgs, "--set-string", component.key+".tag="+baseline)
	}
	require.NoError(t, runCommand(ctx, baselineArgs))
	requireContainerImage(t, ctx, k8sClient, "applications-rp", "applications-rp", "ghcr.io/radius-project/applications-rp:"+baseline)
	requireContainerImage(t, ctx, k8sClient, "ucp", "ucp", "ghcr.io/radius-project/ucpd:"+baseline)
	cli := radcli.NewCLI(t, "")
	_, err = cli.RunCommand(ctx, []string{"group", "create", "kind-radius"})
	require.NoError(t, err)

	options := rp.NewRPTestOptions(t)
	client := secretstore.New(t, options)
	name := fmt.Sprintf("ss-upgrade-%d", time.Now().UnixNano())
	ref := name + "/legacy"
	t.Cleanup(func() {
		cleanup := context.WithoutCancel(ctx)
		assert.NoError(t, client.Delete(cleanup, name))
		assert.NoError(t, k8sClient.CoreV1().Namespaces().Delete(cleanup, name, metav1.DeleteOptions{}))
	})
	require.NoError(t, client.Put(ctx, name, secretstore.Resource(ref, initial)))
	client.RequireValue(t, name, ref, initial)
	before, err := k8sClient.CoreV1().Secrets(name).Get(ctx, "legacy", metav1.GetOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, before.UID)
	record, err := client.API.Get(ctx, client.Scope, name, nil)
	require.NoError(t, err)

	registry, tag := testutil.SetDefault()
	deImage, err := deploymentEngineImageFromEnv()
	require.NoError(t, err)
	args := helmReleaseArgs("upgrade", registry, tag, deImage, map[string]string{
		"preupgrade.enabled":  "false",
		"global.rbac.enabled": "false",
		"global.rbac.dryRun":  "false",
	})
	if cert := os.Getenv("RADIUS_REGISTRY_CERT_FILE"); cert != "" {
		args = append(args, "--set-file", "global.rootCA.cert="+cert)
	}
	require.NoError(t, runCommand(ctx, args))
	requireContainerImage(t, ctx, k8sClient, "applications-rp", "applications-rp", registry+"/applications-rp:"+tag)
	requireContainerImage(t, ctx, k8sClient, "ucp", "ucp", registry+"/ucpd:"+tag)
	persisted, err := client.API.Get(ctx, client.Scope, name, nil)
	require.NoError(t, err)
	require.Equal(t, record.ID, persisted.ID)
	client.RequireValue(t, name, ref, initial)
	afterUpgrade, err := k8sClient.CoreV1().Secrets(name).Get(ctx, "legacy", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, before.UID, afterUpgrade.UID)
	require.Equal(t, before.ResourceVersion, afterUpgrade.ResourceVersion)

	require.NoError(t, client.Patch(ctx, name, secretstore.Resource("", updated)))
	client.RequireValue(t, name, ref, updated)
	beforeDelete, err := k8sClient.CoreV1().Secrets(name).Get(ctx, "legacy", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, before.UID, beforeDelete.UID)
	require.NoError(t, client.Delete(ctx, name))
	client.RequireGone(t, name)
	require.NoError(t, client.Delete(ctx, name))
	retained, err := k8sClient.CoreV1().Secrets(name).Get(ctx, "legacy", metav1.GetOptions{})
	require.NoError(t, err, "pre-upgrade records have no ownership UID; their labeled Secrets must survive")
	require.Equal(t, beforeDelete.UID, retained.UID)
	require.Equal(t, beforeDelete.ResourceVersion, retained.ResourceVersion)
	require.NoError(t, client.Put(ctx, name, secretstore.Resource(ref, "")))
	client.RequireValue(t, name, ref, updated)
	require.NoError(t, client.Delete(ctx, name))
	client.RequireGone(t, name)
	retained, err = k8sClient.CoreV1().Secrets(name).Get(ctx, "legacy", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, beforeDelete.UID, retained.UID)
	require.Equal(t, beforeDelete.ResourceVersion, retained.ResourceVersion)
}
