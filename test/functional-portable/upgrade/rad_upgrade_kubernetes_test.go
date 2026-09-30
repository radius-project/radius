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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/testutil"
)

const (
	// The in-repo chart reports this appVersion, which is what an edge install records.
	edgeAppVersion = "edge"

	// Versioned copies of the in-repo chart. Installed versions are read from the release's
	// appVersion, so these give the upgrade preflight checks real semantic versions to compare.
	upgradeTargetVersion = "0.60.0"
	nextMinorVersion     = "0.61.0"
	nextPatchVersion     = "0.61.1"

	// sentinelValueKey is set at install time and used to verify that `rad upgrade kubernetes`
	// carries stored user values forward. It is observable in the cluster: the chart only
	// renders the key rotation CronJob when this value is true (the chart default).
	sentinelValueKey   = "encryption.rotation.enabled"
	keyRotationCronJob = "radius-key-rotation"

	// overlayValueKey is supplied on a later upgrade to verify that new --set values are
	// layered on top of the stored ones.
	overlayValueKey     = "dashboard.enabled"
	dashboardDeployment = "dashboard"

	ucpDeployment = "ucp"
	ucpContainer  = "ucp"
)

// Test_RadUpgradeKubernetes exercises the `rad upgrade kubernetes` command end to end.
//
// The parent installs Radius from the in-repo (edge) chart with `rad install kubernetes`, and the
// subtests then upgrade that single release in sequence, first to a 0.60.0 copy of the chart and
// then to 0.61.0 and 0.61.1 copies. They share a Helm release and a Kubernetes namespace and so must not run
// in parallel, and each subtest depends on the state the previous one left behind.
func Test_RadUpgradeKubernetes(t *testing.T) {
	ctx := t.Context()
	cli := radcli.NewCLI(t, "")

	k8sClient, err := newKubernetesClient()
	require.NoError(t, err, "Failed to create Kubernetes client")

	deImage, err := deploymentEngineImageFromEnv()
	require.NoError(t, err, "Invalid Deployment Engine image configuration")

	registry, tag := testutil.SetDefault()
	imageArgs := radUpgradeImageArgs(registry, tag, deImage, os.Getenv("RADIUS_REGISTRY_CERT_FILE"))
	ucpImage := fmt.Sprintf("%s/ucpd:%s", registry, tag)

	chartDir := t.TempDir()
	upgradeChart := writeVersionedChart(t, relativeChartPath, filepath.Join(chartDir, upgradeTargetVersion), upgradeTargetVersion)
	nextChart := writeVersionedChart(t, relativeChartPath, filepath.Join(chartDir, nextMinorVersion), nextMinorVersion)
	patchChart := writeVersionedChart(t, relativeChartPath, filepath.Join(chartDir, nextPatchVersion), nextPatchVersion)

	cleanupAndWait(t, ctx, k8sClient)

	t.Log("Verifying upgrade fails when Radius is not installed")
	out, err := cli.RunCommand(ctx, []string{"upgrade", "kubernetes", "--chart", upgradeChart, "--version", upgradeTargetVersion})
	require.Error(t, err, "rad upgrade kubernetes should fail when Radius is not installed")
	require.Contains(t, out, "the Radius control plane is not currently installed")

	t.Log("Installing Radius from the in-repo chart with rad install kubernetes")
	installArgs := append([]string{
		"install", "kubernetes",
		"--chart", relativeChartPath,
		"--skip-contour-install",
		"--set", sentinelValueKey + "=false",
	}, imageArgs...)
	out, err = cli.RunCommand(ctx, installArgs)
	require.NoErrorf(t, err, "rad install kubernetes failed: %s", out)

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupCommandTimeout)
		defer cancel()
		helmUninstall(t, cleanupCtx)
	})

	requireRelease(t, ctx, edgeAppVersion)
	requireCronJobExists(t, ctx, k8sClient, keyRotationCronJob, false)

	upgrade := func(t *testing.T, args ...string) (string, error) {
		t.Helper()
		return cli.RunCommand(ctx, append([]string{"upgrade", "kubernetes"}, args...))
	}

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"RejectsConflictingPreflightFlags", func(t *testing.T) {
			before := latestRevision(t, ctx)
			out, err := upgrade(t, "--skip-preflight", "--preflight-only")
			require.Error(t, err)
			require.Contains(t, out, "cannot specify both --skip-preflight and --preflight-only")
			require.Equal(t, before, latestRevision(t, ctx), "A rejected upgrade must not create a Helm revision")
		}},
		{"PreflightOnlyFromEdge", func(t *testing.T) {
			before := latestRevision(t, ctx)
			out, err := upgrade(t, "--preflight-only", "--chart", upgradeChart, "--version", upgradeTargetVersion)
			require.NoErrorf(t, err, "Preflight checks should pass when upgrading from an edge install: %s", out)
			require.Contains(t, out, "Current Radius version: "+edgeAppVersion)
			require.Contains(t, out, "Target Radius version: "+upgradeTargetVersion)
			require.Contains(t, out, "All preflight checks passed")
			require.Contains(t, out, "Upgrade was not performed due to --preflight-only flag")
			require.Equal(t, before, latestRevision(t, ctx), "--preflight-only must not create a Helm revision")
		}},
		{"UpgradePreservesStoredValues", func(t *testing.T) {
			before := latestRevision(t, ctx)
			out, err := upgrade(t, "--chart", upgradeChart, "--version", upgradeTargetVersion)
			require.NoErrorf(t, err, "rad upgrade kubernetes failed: %s", out)
			require.Contains(t, out, "All preflight checks passed")
			require.Contains(t, out, "Radius upgrade completed successfully")
			require.Equal(t, before+1, latestRevision(t, ctx), "Upgrade should create exactly one Helm revision")
			requireRelease(t, ctx, upgradeTargetVersion)

			values := helmUserValues(t, ctx)
			requireValue(t, values, sentinelValueKey, false)
			requireValue(t, values, "ucp.image", registry+"/ucpd")
			requireCronJobExists(t, ctx, k8sClient, keyRotationCronJob, false)
			requireContainerImage(t, ctx, k8sClient, ucpDeployment, ucpContainer, ucpImage)
			requireDeploymentEngineImage(t, ctx, k8sClient, deImage, "rad upgrade kubernetes")
		}},
		{"RejectsInvalidVersionJumps", func(t *testing.T) {
			for _, tc := range []struct {
				name, version, message string
			}{
				{"Downgrade", "0.59.0", "Downgrading is not supported"},
				{"SameVersion", upgradeTargetVersion, "Target version is the same as current version"},
				{"SkippedMinorVersion", "0.62.0", "Only incremental version upgrades are supported"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					before := latestRevision(t, ctx)
					out, err := upgrade(t, "--chart", nextChart, "--version", tc.version)
					require.Errorf(t, err, "Upgrade from %s to %s should be rejected", upgradeTargetVersion, tc.version)
					require.Contains(t, out, "Current Radius version: "+upgradeTargetVersion)
					require.Contains(t, out, tc.message)
					require.Equal(t, before, latestRevision(t, ctx), "A rejected upgrade must not create a Helm revision")
					requireRelease(t, ctx, upgradeTargetVersion)
				})
			}
		}},
		{"SkipPreflightOverlaysNewValues", func(t *testing.T) {
			before := latestRevision(t, ctx)
			out, err := upgrade(t, "--skip-preflight", "--chart", nextChart, "--version", nextMinorVersion,
				"--set", overlayValueKey+"=false")
			require.NoErrorf(t, err, "rad upgrade kubernetes failed: %s", out)
			require.NotContains(t, out, "Running pre-flight checks", "--skip-preflight must not run preflight checks")
			require.Equal(t, before+1, latestRevision(t, ctx), "Upgrade should create exactly one Helm revision")
			requireRelease(t, ctx, nextMinorVersion)

			values := helmUserValues(t, ctx)
			requireValue(t, values, sentinelValueKey, false)
			requireValue(t, values, overlayValueKey, false)
			requireCronJobExists(t, ctx, k8sClient, keyRotationCronJob, false)
			requireDeploymentExists(t, ctx, k8sClient, dashboardDeployment, false)
			requireContainerImage(t, ctx, k8sClient, ucpDeployment, ucpContainer, ucpImage)
		}},
		{"ResetValuesDiscardsStoredValues", func(t *testing.T) {
			before := latestRevision(t, ctx)
			args := append([]string{"--reset-values", "--chart", patchChart, "--version", nextPatchVersion}, imageArgs...)
			out, err := upgrade(t, args...)
			require.NoErrorf(t, err, "rad upgrade kubernetes --reset-values failed: %s", out)
			require.Equal(t, before+1, latestRevision(t, ctx), "Upgrade should create exactly one Helm revision")
			requireRelease(t, ctx, nextPatchVersion)

			values := helmUserValues(t, ctx)
			_, found := lookupValue(values, sentinelValueKey)
			require.False(t, found, "--reset-values should discard the stored %s value", sentinelValueKey)
			_, found = lookupValue(values, overlayValueKey)
			require.False(t, found, "--reset-values should discard the stored %s value", overlayValueKey)
			requireValue(t, values, "ucp.image", registry+"/ucpd")
			requireCronJobExists(t, ctx, k8sClient, keyRotationCronJob, true)
			requireDeploymentExists(t, ctx, k8sClient, dashboardDeployment, true)
			requireContainerImage(t, ctx, k8sClient, ucpDeployment, ucpContainer, ucpImage)
			requireDeploymentEngineImage(t, ctx, k8sClient, deImage, "rad upgrade kubernetes --reset-values")
		}},
		{"RollbackListRevisions", func(t *testing.T) {
			history := helmHistory(t, ctx)
			out, err := cli.RunCommand(ctx, []string{"rollback", "kubernetes", "--list-revisions"})
			require.NoErrorf(t, err, "rad rollback kubernetes --list-revisions failed: %s", out)
			require.Contains(t, out, "REVISION")
			for _, revision := range history {
				require.Regexpf(t, fmt.Sprintf(`(?m)^%d\s+%s\s`, revision.Revision, regexp.QuoteMeta(revision.chartVersion())), out,
					"Revision %d (%s) missing from --list-revisions output", revision.Revision, revision.Chart)
			}
			require.Equal(t, history, helmHistory(t, ctx), "--list-revisions must not change the release history")
		}},
	}

	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}

// radUpgradeImageArgs pins every image the chart renders so the result does not depend on the
// chart's appVersion, which the versioned chart copies change. The Radius images come from the
// build under test. The Deployment Engine and dashboard fall back to the tags the edge chart
// renders ("latest") when no override is configured.
func radUpgradeImageArgs(registry, tag string, deImage deploymentEngineImage, rootCACertFile string) []string {
	var args []string
	for _, component := range []struct{ key, image string }{
		{"controller", "controller"},
		{"rp", "applications-rp"},
		{"dynamicrp", "dynamic-rp"},
		{"ucp", "ucpd"},
		{"bicep", "bicep"},
		{"preupgrade", "pre-upgrade"},
	} {
		args = append(args, "--set", fmt.Sprintf("%s.image=%s/%s,%s.tag=%s", component.key, registry, component.image, component.key, tag))
	}
	if deImage.repository != "" {
		args = append(args, "--set", fmt.Sprintf("de.image=%s,de.tag=%s", deImage.repository, deImage.tag))
	} else {
		args = append(args, "--set", "de.tag=latest")
	}
	args = append(args, "--set", "dashboard.tag=latest")
	if rootCACertFile != "" {
		args = append(args, "--set-file", "global.rootCA.cert="+rootCACertFile)
	}
	return args
}

var (
	chartVersionPattern    = regexp.MustCompile(`(?m)^version:.*$`)
	chartAppVersionPattern = regexp.MustCompile(`(?m)^appVersion:.*$`)
)

// writeVersionedChart copies the chart at src to dst and sets both its version and appVersion.
func writeVersionedChart(t *testing.T, src, dst, version string) string {
	t.Helper()
	require.NoError(t, os.CopyFS(dst, os.DirFS(src)), "Failed to copy chart %s to %s", src, dst)

	chartFile := filepath.Join(dst, "Chart.yaml")
	content, err := os.ReadFile(chartFile)
	require.NoError(t, err)

	updated, err := setChartVersion(string(content), version)
	require.NoError(t, err, "Failed to set version of chart %s", chartFile)
	require.NoError(t, os.WriteFile(chartFile, []byte(updated), 0o644))
	return dst
}

// setChartVersion rewrites the top-level version and appVersion fields of a Chart.yaml.
func setChartVersion(chartYAML, version string) (string, error) {
	if !chartVersionPattern.MatchString(chartYAML) || !chartAppVersionPattern.MatchString(chartYAML) {
		return "", fmt.Errorf("Chart.yaml must declare both version and appVersion")
	}
	chartYAML = chartVersionPattern.ReplaceAllLiteralString(chartYAML, fmt.Sprintf("version: '%s'", version))
	return chartAppVersionPattern.ReplaceAllLiteralString(chartYAML, fmt.Sprintf("appVersion: '%s'", version)), nil
}

// helmRevision is the subset of "helm history --output json" this test reads.
type helmRevision struct {
	Revision   int    `json:"revision"`
	Status     string `json:"status"`
	Chart      string `json:"chart"`
	AppVersion string `json:"app_version"`
}

// chartVersion returns the chart version from Helm's "<name>-<version>" chart column.
func (r helmRevision) chartVersion() string {
	return strings.TrimPrefix(r.Chart, helmReleaseName+"-")
}

func helmHistory(t *testing.T, ctx context.Context) []helmRevision {
	t.Helper()
	var history []helmRevision
	helmJSON(t, ctx, &history, "history", helmReleaseName, "--namespace", radiusNamespace, "--output", "json")
	require.NotEmpty(t, history, "Helm release %q has no history", helmReleaseName)
	return history
}

func latestRevision(t *testing.T, ctx context.Context) int {
	t.Helper()
	history := helmHistory(t, ctx)
	return slices.MaxFunc(history, func(a, b helmRevision) int { return a.Revision - b.Revision }).Revision
}

// requireRelease verifies the latest release revision is deployed with the given app version.
func requireRelease(t *testing.T, ctx context.Context, appVersion string) {
	t.Helper()
	history := helmHistory(t, ctx)
	latest := slices.MaxFunc(history, func(a, b helmRevision) int { return a.Revision - b.Revision })
	require.Equal(t, helmStatusDeployed, latest.Status, "Latest revision %d should be deployed", latest.Revision)
	require.Equal(t, appVersion, latest.AppVersion, "Latest revision %d has an unexpected app version", latest.Revision)
}

// helmUserValues returns the user-supplied values stored on the Radius release.
func helmUserValues(t *testing.T, ctx context.Context) map[string]any {
	t.Helper()
	values := map[string]any{}
	helmJSON(t, ctx, &values, "get", "values", helmReleaseName, "--namespace", radiusNamespace, "--output", "json")
	return values
}

// helmJSON runs a helm command and decodes its stdout. Only stdout is decoded because Helm writes
// diagnostics, such as kubeconfig permission warnings, to stderr.
func helmJSON(t *testing.T, ctx context.Context, into any, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, "helm", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	require.NoErrorf(t, err, "helm %s failed: %s", strings.Join(args, " "), stderr.String())
	require.NoErrorf(t, json.Unmarshal(stdout, into), "Failed to decode output of helm %s: %s", strings.Join(args, " "), stdout)
}

// lookupValue finds a dotted key such as "encryption.rotation.enabled" in nested Helm values.
func lookupValue(values map[string]any, key string) (any, bool) {
	var current any = values
	for part := range strings.SplitSeq(key, ".") {
		node, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		if current, ok = node[part]; !ok {
			return nil, false
		}
	}
	return current, true
}

func requireValue(t *testing.T, values map[string]any, key string, expected any) {
	t.Helper()
	actual, found := lookupValue(values, key)
	require.Truef(t, found, "Stored Helm values are missing %s: %v", key, values)
	require.Equalf(t, expected, actual, "Stored Helm value %s", key)
}

func requireCronJobExists(t *testing.T, ctx context.Context, client kubernetes.Interface, name string, exists bool) {
	t.Helper()
	_, err := client.BatchV1().CronJobs(radiusNamespace).Get(ctx, name, metav1.GetOptions{})
	requireExistence(t, err, "CronJob", name, exists)
}

func requireDeploymentExists(t *testing.T, ctx context.Context, client kubernetes.Interface, name string, exists bool) {
	t.Helper()
	_, err := client.AppsV1().Deployments(radiusNamespace).Get(ctx, name, metav1.GetOptions{})
	requireExistence(t, err, "Deployment", name, exists)
}

func requireExistence(t *testing.T, err error, kind, name string, exists bool) {
	t.Helper()
	if exists {
		require.NoErrorf(t, err, "%s %s/%s should exist", kind, radiusNamespace, name)
		return
	}
	require.Truef(t, apierrors.IsNotFound(err), "%s %s/%s should not exist, got err=%v", kind, radiusNamespace, name, err)
}

func requireContainerImage(t *testing.T, ctx context.Context, client kubernetes.Interface, deploymentName, containerName, expected string) {
	t.Helper()
	deployment, err := client.AppsV1().Deployments(radiusNamespace).Get(ctx, deploymentName, metav1.GetOptions{})
	require.NoErrorf(t, err, "Failed to get Deployment %s/%s", radiusNamespace, deploymentName)
	index := slices.IndexFunc(deployment.Spec.Template.Spec.Containers, func(c corev1.Container) bool { return c.Name == containerName })
	require.GreaterOrEqualf(t, index, 0, "Deployment %s/%s has no %s container", radiusNamespace, deploymentName, containerName)
	require.Equal(t, expected, deployment.Spec.Template.Spec.Containers[index].Image)
}

func Test_radUpgradeImageArgs(t *testing.T) {
	components := []string{
		"--set", "controller.image=radius-registry:5000/controller,controller.tag=pr-1",
		"--set", "rp.image=radius-registry:5000/applications-rp,rp.tag=pr-1",
		"--set", "dynamicrp.image=radius-registry:5000/dynamic-rp,dynamicrp.tag=pr-1",
		"--set", "ucp.image=radius-registry:5000/ucpd,ucp.tag=pr-1",
		"--set", "bicep.image=radius-registry:5000/bicep,bicep.tag=pr-1",
		"--set", "preupgrade.image=radius-registry:5000/pre-upgrade,preupgrade.tag=pr-1",
	}
	for _, tc := range []struct {
		name    string
		deImage deploymentEngineImage
		cert    string
		want    []string
	}{
		{
			name: "default Deployment Engine",
			want: append(slices.Clone(components), "--set", "de.tag=latest", "--set", "dashboard.tag=latest"),
		},
		{
			name:    "candidate Deployment Engine and root CA",
			deImage: deploymentEngineImage{repository: "ghcr.io/radius-project/deployment-engine", tag: "candidate"},
			cert:    "/tmp/certs/client.crt",
			want: append(slices.Clone(components),
				"--set", "de.image=ghcr.io/radius-project/deployment-engine,de.tag=candidate",
				"--set", "dashboard.tag=latest",
				"--set-file", "global.rootCA.cert=/tmp/certs/client.crt"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, radUpgradeImageArgs("radius-registry:5000", "pr-1", tc.deImage, tc.cert))
		})
	}
}

func Test_setChartVersion(t *testing.T) {
	chart := "apiVersion: v2\nname: radius\n# version is replaced\nversion: '0.42.42-dev'\nappVersion: 'edge'\nkeywords:\n  - version: nested\n"
	updated, err := setChartVersion(chart, "0.60.0")
	require.NoError(t, err)
	require.Equal(t, "apiVersion: v2\nname: radius\n# version is replaced\nversion: '0.60.0'\nappVersion: '0.60.0'\nkeywords:\n  - version: nested\n", updated)

	_, err = setChartVersion("apiVersion: v2\nversion: '1.0.0'\n", "0.60.0")
	require.Error(t, err)
}

func Test_writeVersionedChart(t *testing.T) {
	dst := writeVersionedChart(t, relativeChartPath, filepath.Join(t.TempDir(), "chart"), upgradeTargetVersion)

	content, err := os.ReadFile(filepath.Join(dst, "Chart.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(content), "version: '"+upgradeTargetVersion+"'")
	require.Contains(t, string(content), "appVersion: '"+upgradeTargetVersion+"'")
	require.FileExists(t, filepath.Join(dst, "values.yaml"))
	require.DirExists(t, filepath.Join(dst, "templates"))
}

func Test_lookupValue(t *testing.T) {
	values := map[string]any{
		"encryption": map[string]any{"rotation": map[string]any{"enabled": false}},
		"ucp":        map[string]any{"image": "radius-registry:5000/ucpd"},
	}
	for _, tc := range []struct {
		key   string
		want  any
		found bool
	}{
		{"encryption.rotation.enabled", false, true},
		{"ucp.image", "radius-registry:5000/ucpd", true},
		{"ucp.tag", nil, false},
		{"ucp.image.name", nil, false},
		{"dashboard.enabled", nil, false},
	} {
		t.Run(tc.key, func(t *testing.T) {
			got, found := lookupValue(values, tc.key)
			require.Equal(t, tc.found, found)
			require.Equal(t, tc.want, got)
		})
	}
}
