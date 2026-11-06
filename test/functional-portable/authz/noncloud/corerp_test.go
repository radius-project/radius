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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/test/functional-portable/corerp"
	"github.com/radius-project/radius/test/radcli"
)

// Keep this test serial: it owns mode transitions on the disposable test cluster.
// Go defers parallel tests until serial tests and their cleanups have finished.
func Test_ApplicationsCoreRegression(t *testing.T) {
	expectedInstalledMode(t)
	values := installedValues(t)
	global, ok := values["global"].(map[string]any)
	require.True(t, ok)
	rbac, ok := global["rbac"].(map[string]any)
	require.True(t, ok)
	enabled, ok := rbac["enabled"].(bool)
	require.True(t, ok)
	dryRun, ok := rbac["dryRun"].(bool)
	require.True(t, ok)
	chart := os.Getenv("RADIUS_CHART_LOCATION")
	if chart == "" {
		chart = filepath.Join("deploy", "Chart")
	}
	if !filepath.IsAbs(chart) {
		chart = filepath.Join("..", "..", "..", "..", chart)
	}
	cli := radcli.NewCLI(t, "")
	currentEnabled, currentDryRun, ready := enabled, dryRun, true
	install := func(ctx context.Context, enabled, dryRun bool) error {
		if ready && enabled == currentEnabled && dryRun == currentDryRun {
			return nil
		}
		ready = false
		_, err := cli.RunCommand(ctx, []string{
			"install", "kubernetes", "--reinstall", "--chart", chart,
			"--set", fmt.Sprintf("global.rbac.enabled=%t", enabled),
			"--set", fmt.Sprintf("global.rbac.dryRun=%t", dryRun),
		})
		if err != nil {
			return err
		}
		for _, c := range components {
			output, err := exec.CommandContext(ctx, "kubectl", "rollout", "status",
				"deployment/"+c.deployment, "-n", radiusNamespace, "--timeout=5m").CombinedOutput()
			if err != nil {
				return fmt.Errorf("waiting for %s: %w: %s", c.deployment, err, output)
			}
			currentEnabled, currentDryRun, ready = enabled, dryRun, true
		}
		return nil
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Minute)
		defer cancel()
		assert.NoError(t, install(ctx, enabled, dryRun), "restore original authorization flags")
	})
	references := secretStoreReferences(t)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Minute)
		defer cancel()
		assert.NoError(t, install(ctx, false, false), "allow protected fixture cleanup")
	})

	// dryRun persists real records before enforcement; off also exercises the
	// legacy behavior and leaves the installation ready for fixture cleanup.
	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce, authz.ModeOff} {
		require.NoError(t, install(t.Context(), mode == authz.ModeEnforce, mode == authz.ModeDryRun))
		require.Equal(t, mode, expectedMode(installedValues(t)))
		t.Run(string(mode), func(t *testing.T) {
			t.Run("allowed", corerp.RunRegression)
			t.Run("protectedReferences", func(t *testing.T) { references(t, mode) })
		})
	}
}
