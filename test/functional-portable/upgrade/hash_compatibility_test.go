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
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	ucpv1alpha1 "github.com/radius-project/radius/pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/radius-project/radius/pkg/hashutil"
	radiuskubernetes "github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/trackedresource"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	hashApplication = "hash-upgrade-app"
	hashEnvironment = "hash-upgrade-env"
	hashStateful    = "hash-upgrade-state"
	hashDuring      = "hash-upgrade-during"
	hashModuleNS    = "radius-hash-upgrade-fixtures"
	hashTemplate    = "testdata/hash-compatibility.bicep"

	// hashMigrationTimeout covers the migration's initial delay and a few retries after the upgrade.
	hashMigrationTimeout = 10 * time.Minute
)

// hashMigrationKeys are the status keys every component records in the hashmigration.ConfigMapName ConfigMap.
var hashMigrationKeys = []string{
	"ucp.trackedResources",
	"ucp.datastore",
	"applications-rp.terraformState",
	"applications-rp.containerSecretHash",
	"dynamic-rp.terraformState",
}

//go:embed testdata/hash-compatibility.tf
var hashTerraformModule []byte

// Test_HashCompatibilityUpgrade uses real, distinct development images and historical-format
// fixtures. It does not substitute version labels for a SHA-1-only source binary.
func Test_HashCompatibilityUpgrade(t *testing.T) {
	baselineRegistry, baselineTag := os.Getenv("RADIUS_HASH_BASELINE_REGISTRY"), os.Getenv("RADIUS_HASH_BASELINE_TAG")
	if baselineRegistry == "" && baselineTag == "" {
		t.Skip("set RADIUS_HASH_BASELINE_REGISTRY and RADIUS_HASH_BASELINE_TAG to run the isolated hash-compatibility upgrade")
	}
	require.NotEmpty(t, baselineRegistry)
	require.NotEmpty(t, baselineTag)
	registry, tag := testutil.SetDefault()
	require.NotEqual(t, baselineRegistry+":"+baselineTag, registry+":"+tag, "baseline and candidate must be distinct builds")

	ctx := t.Context()
	kube, err := newKubernetesClient()
	require.NoError(t, err)
	deImage, err := deploymentEngineImageFromEnv()
	require.NoError(t, err)
	require.NoError(t, cleanupHashPreflight(ctx, kube))
	cleanupAndWait(t, ctx, kube)
	values := map[string]string{
		"dashboard.enabled":                  "false",
		"encryption.rotation.enabled":        "false",
		"preupgrade.enabled":                 "true",
		"preupgrade.checks.version":          "true",
		"preupgrade.ttlSecondsAfterFinished": "60",
	}
	require.NoError(t, helmInstall(ctx, baselineRegistry, baselineTag, deImage, values))
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupCommandTimeout)
		defer cancel()
		helmUninstall(t, cleanupCtx)
		require.NoError(t, cleanupHashPreflight(cleanupCtx, kube))
	})
	cli := radcli.NewCLI(t, "")
	for _, args := range [][]string{{"workspace", "create", "kubernetes", "--force"}, {"group", "create", "hash-upgrade"}, {"group", "switch", "hash-upgrade"}} {
		out, err := cli.RunCommand(ctx, args)
		require.NoErrorf(t, err, "%v: %s", args, out)
	}
	moduleServer := createHashModuleServer(t, ctx, kube)
	workloadImage := os.Getenv("RADIUS_HASH_WORKLOAD_IMAGE")
	if workloadImage == "" {
		workloadImage = "alpine:3.24.2"
	}
	params := []string{"moduleServer=" + moduleServer, "workloadImage=" + workloadImage}
	resourceSet := &validation.RPResourceSet{Resources: []validation.RPResource{
		{Name: hashEnvironment, Type: validation.EnvironmentsResource},
		{Name: hashApplication, Type: validation.ApplicationsResource},
		{Name: "hash-upgrade-consumer", Type: validation.ContainersResource, App: hashApplication},
		{Name: hashStateful, Type: validation.ExtendersResource, App: hashApplication},
	}}
	var legacyObjects map[string]types.UID
	var before hashWorkloads
	var initialState, movedState, beforeUpdate hashState
	var legacySecret, duringSecret string
	var scope string
	test := rp.NewRPTest(t, hashApplication, nil)
	test.RunSerial, test.FastCleanup = true, false
	require.NoError(t, ucpv1alpha1.AddToScheme(test.Options.Client.Scheme()))
	scope = test.Options.Workspace.Scope
	stateID := resources.MustParse(scope + "/providers/Applications.Core/extenders/" + hashStateful)
	duringID := resources.MustParse(scope + "/providers/Applications.Core/extenders/" + hashDuring)
	currentSecret := hashStateSecret(stateID, false)
	legacySecret = hashStateSecret(stateID, true)
	duringSecret = hashStateSecret(duringID, false)
	owned := map[string]bool{}
	for _, resource := range resourceSet.Resources {
		owned[strings.ToLower(scope+"/providers/"+resource.Type+"/"+resource.Name)] = true
	}
	test.Steps = []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(hashTemplate, params...), RPResources: resourceSet,
			SkipKubernetesOutputResourceValidation: true, SkipObjectValidation: true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				before = captureHashWorkloads(t, ctx, kube)
				initialState = readHashState(t, ctx, kube, currentSecret)
				require.Equal(t, "before", initialState.Input)
				waitHashTrackingIdle(t, ctx, test.Options.Client, owned)
				restore := pauseHashControlPlane(t, ctx, kube)
				defer func() {
					restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupCommandTimeout)
					defer cancel()
					if err := restore(restoreCtx); err != nil {
						t.Errorf("restoring control plane: %v", err)
					}
				}()
				legacyObjects = seedHashIdentities(t, ctx, test.Options.Client, owned)
				seedHashState(t, ctx, kube, currentSecret, legacySecret)
				seededState := readHashState(t, ctx, kube, legacySecret)
				require.Equal(t, initialState.Lineage, seededState.Lineage)
				require.Equal(t, initialState.ID, seededState.ID)
				require.Equal(t, initialState.Input, seededState.Input)
				initialState = seededState
				require.NoError(t, restore(ctx))
				requireHashWorkloads(t, before, captureHashWorkloads(t, ctx, kube))
				sourceImage := hashUCPImage(t, ctx, kube)
				t.Logf("baseline=%s/ucpd:%s, imageID=%s; candidate=%s/ucpd:%s; chart=edge; fixture=pre-862fc3d5a identity format",
					baselineRegistry, baselineTag, sourceImage, registry, tag)
				duringParams := append(append([]string{}, params...), "createDuring=true")
				writes, err := writeAcrossHashUpgrade(ctx, cli, duringParams, func() error {
					return helmUpgrade(ctx, registry, tag, deImage, values)
				})
				require.NoError(t, err)
				require.Positive(t, writes)
				targetImage := hashUCPImage(t, ctx, kube)
				require.NotEqual(t, sourceImage, targetImage, "the candidate must replace the baseline binary image")
				t.Logf("candidate imageID=%s; successful concurrent deployments=%d", targetImage, writes)
				requireHashWorkloads(t, before, captureHashWorkloads(t, ctx, kube))

				// The candidate migrates every legacy identity in the background without touching workloads.
				statuses := waitHashMigration(t, ctx, kube)
				for _, key := range hashMigrationKeys {
					t.Logf("migration %s: %+v", key, statuses[key])
				}
				requireHashMigrated(t, ctx, test.Options.Client, legacyObjects, owned)
				movedState = requireHashStateMoved(t, ctx, kube, initialState, legacySecret, currentSecret)
				requireHashSecretHashes(t, ctx, kube)
				requireHashWorkloads(t, before, captureHashWorkloads(t, ctx, kube))
				for _, id := range []resources.ID{duringID, trackedresource.IDFor(duringID)} {
					name, err := hashFixtureObjectName(id, false)
					require.NoError(t, err)
					require.NoError(t, test.Options.Client.Get(ctx, runtimeclient.ObjectKey{Namespace: radiusNamespace, Name: name}, &ucpv1alpha1.Resource{}))
				}
				require.NotEqual(t, initialState.Lineage, readHashState(t, ctx, kube, duringSecret).Lineage)
			},
		},
		{
			Executor: step.NewDeployExecutor(hashTemplate, params...), SkipResourceDeletion: true,
			SkipKubernetesOutputResourceValidation: true, SkipObjectValidation: true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				requireHashWorkloads(t, before, captureHashWorkloads(t, ctx, kube))
				requireHashMigrated(t, ctx, test.Options.Client, legacyObjects, owned)
				beforeUpdate = readHashState(t, ctx, kube, currentSecret)
				requireHashState(t, movedState, beforeUpdate, "before")
			},
		},
		{
			Executor: step.NewDeployExecutor(hashTemplate, append(append([]string{}, params...), "message=updated")...), SkipResourceDeletion: true,
			SkipKubernetesOutputResourceValidation: true, SkipObjectValidation: true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				state := readHashState(t, ctx, kube, currentSecret)
				requireHashState(t, beforeUpdate, state, "updated")
				require.Greater(t, state.Serial, beforeUpdate.Serial)
				requireHashWorkloads(t, before, captureHashWorkloads(t, ctx, kube))
			},
		},
	}
	test.PostDeleteVerify = func(ctx context.Context, t *testing.T, test rp.RPTest) {
		for _, name := range []string{legacySecret, currentSecret, duringSecret} {
			_, err := kube.CoreV1().Secrets(radiusNamespace).Get(ctx, name, metav1.GetOptions{})
			require.True(t, apierrors.IsNotFound(err), "state secret %s must be deleted", name)
		}
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			for name := range legacyObjects {
				err := test.Options.Client.Get(ctx, runtimeclient.ObjectKey{Namespace: radiusNamespace, Name: name}, &ucpv1alpha1.Resource{})
				assert.True(c, apierrors.IsNotFound(err), "legacy object %s must be deleted", name)
			}
			for original := range owned {
				name, err := hashFixtureObjectName(resources.MustParse(original), false)
				if assert.NoError(c, err) {
					err = test.Options.Client.Get(ctx, runtimeclient.ObjectKey{Namespace: radiusNamespace, Name: name}, &ucpv1alpha1.Resource{})
					assert.True(c, apierrors.IsNotFound(err), "migrated object %s must be deleted", name)
				}
			}
		}, cleanupTimeout, jobPollInterval)
	}
	test.Test(t)
}

func cleanupHashPreflight(ctx context.Context, kube kubernetes.Interface) error {
	job, err := kube.BatchV1().Jobs(radiusNamespace).Get(ctx, preUpgradeJobName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, condition := range job.Status.Conditions {
		if condition.Status == corev1.ConditionTrue && (condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) {
			return kube.BatchV1().Jobs(radiusNamespace).Delete(ctx, job.Name, metav1.DeleteOptions{
				Preconditions: &metav1.Preconditions{UID: &job.UID, ResourceVersion: &job.ResourceVersion},
			})
		}
	}
	return fmt.Errorf("refusing to remove unfinished pre-upgrade job %s/%s", radiusNamespace, job.Name)
}

func createHashModuleServer(t *testing.T, ctx context.Context, kube kubernetes.Interface) string {
	t.Helper()
	_, err := kube.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{Name: hashModuleNS}, metav1.CreateOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupCommandTimeout)
		defer cancel()
		if err := kube.CoreV1().Namespaces().Delete(cleanupCtx, hashModuleNS, metav1.DeleteOptions{}); err != nil {
			t.Errorf("deleting fixture namespace: %v", err)
		}
	})
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	file, err := writer.Create("modules/main.tf")
	require.NoError(t, err)
	_, err = file.Write(hashTerraformModule)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	_, err = kube.CoreV1().ConfigMaps(hashModuleNS).Create(ctx, &corev1.ConfigMap{
		Name: "tf-module-server-content", Namespace: hashModuleNS, BinaryData: map[string][]byte{"identity.zip": archive.Bytes()},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	require.NoError(t, runCommand(ctx, []string{"kubectl", "apply", "-n", hashModuleNS, "-f", filepath.Join("..", "..", "..", "deploy", "tf-module-server", "resources.yaml")}))
	require.NoError(t, runCommand(ctx, []string{"kubectl", "rollout", "status", "deployment/tf-module-server", "-n", hashModuleNS, "--timeout=240s"}))
	return "http://tf-module-server." + hashModuleNS + ".svc.cluster.local"
}

type hashPod struct {
	Owner    types.UID
	Restarts map[string]int32
}

type hashDeployment struct {
	UID      types.UID
	Template corev1.PodTemplateSpec
}

type hashWorkloads struct {
	Deployments map[string]hashDeployment
	Pods        map[types.UID]hashPod
}

func captureHashWorkloads(t *testing.T, ctx context.Context, kube kubernetes.Interface) hashWorkloads {
	t.Helper()
	var result hashWorkloads
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		deployments, err := kube.AppsV1().Deployments(hashApplication).List(ctx, metav1.ListOptions{})
		if !assert.NoError(c, err) || !assert.NotEmpty(c, deployments.Items) {
			return
		}
		result = hashWorkloads{Deployments: map[string]hashDeployment{}, Pods: map[types.UID]hashPod{}}
		hasSecretHash := false
		for _, deployment := range deployments.Items {
			assert.Equal(c, *deployment.Spec.Replicas, deployment.Status.ReadyReplicas)
			result.Deployments[deployment.Name] = hashDeployment{deployment.UID, deployment.Spec.Template}
			hasSecretHash = hasSecretHash || deployment.Spec.Template.Annotations["radapp.io/secret-hash"] != ""
		}
		assert.True(c, hasSecretHash, "the fixture must exercise the secret-hash pod annotation")
		pods, err := kube.CoreV1().Pods(hashApplication).List(ctx, metav1.ListOptions{})
		if !assert.NoError(c, err) || !assert.NotEmpty(c, pods.Items) {
			return
		}
		for _, pod := range pods.Items {
			assert.Nil(c, pod.DeletionTimestamp)
			assert.NotEmpty(c, pod.Status.ContainerStatuses)
			item := hashPod{Restarts: map[string]int32{}}
			for _, owner := range pod.OwnerReferences {
				if owner.Kind == "ReplicaSet" {
					item.Owner = owner.UID
				}
			}
			assert.NotEmpty(c, item.Owner)
			for _, status := range pod.Status.ContainerStatuses {
				assert.True(c, status.Ready)
				item.Restarts[status.Name] = status.RestartCount
			}
			result.Pods[pod.UID] = item
		}
	}, cleanupCommandTimeout, jobPollInterval)
	return result
}

func requireHashWorkloads(t *testing.T, before, after hashWorkloads) {
	t.Helper()
	require.Equal(t, before, after, "unchanged workloads must retain pod, ReplicaSet, template, and restart identities")
}

func hashUCPImage(t *testing.T, ctx context.Context, kube kubernetes.Interface) string {
	t.Helper()
	var image string
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		pods, err := kube.CoreV1().Pods(radiusNamespace).List(ctx, metav1.ListOptions{LabelSelector: radiusPodSelector})
		if !assert.NoError(c, err) {
			return
		}
		image = ""
		count := 0
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp != nil {
				continue
			}
			for _, status := range pod.Status.ContainerStatuses {
				if status.Name == ucpContainer && status.Ready {
					image = status.ImageID
					count++
				}
			}
		}
		assert.Equal(c, 1, count)
		assert.NotEmpty(c, image)
	}, cleanupCommandTimeout, jobPollInterval)
	return image
}

func pauseHashControlPlane(t *testing.T, ctx context.Context, kube kubernetes.Interface) func(context.Context) error {
	t.Helper()
	deployments, err := kube.AppsV1().Deployments(radiusNamespace).List(ctx, metav1.ListOptions{LabelSelector: radiusPodSelector})
	require.NoError(t, err)
	require.NotEmpty(t, deployments.Items)
	replicas := map[string]int32{}
	for _, deployment := range deployments.Items {
		replicas[deployment.Name] = *deployment.Spec.Replicas
	}
	restore := func(ctx context.Context) error {
		var failures []error
		for name, count := range replicas {
			scale, err := kube.AppsV1().Deployments(radiusNamespace).GetScale(ctx, name, metav1.GetOptions{})
			if err == nil && scale.Spec.Replicas != count {
				scale.Spec.Replicas = count
				_, err = kube.AppsV1().Deployments(radiusNamespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
			}
			if err != nil {
				failures = append(failures, fmt.Errorf("restore %s: %w", name, err))
			}
		}
		return errors.Join(failures...)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupCommandTimeout)
		defer cancel()
		if err := restore(cleanupCtx); err != nil {
			t.Errorf("restoring paused control plane: %v", err)
		}
	})
	for name := range replicas {
		scale, err := kube.AppsV1().Deployments(radiusNamespace).GetScale(ctx, name, metav1.GetOptions{})
		require.NoError(t, err)
		scale.Spec.Replicas = 0
		_, err = kube.AppsV1().Deployments(radiusNamespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
		require.NoError(t, err)
	}
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		pods, err := kube.CoreV1().Pods(radiusNamespace).List(ctx, metav1.ListOptions{LabelSelector: radiusPodSelector})
		if !assert.NoError(c, err) {
			return
		}
		for _, pod := range pods.Items {
			for _, owner := range pod.OwnerReferences {
				assert.NotEqual(c, "ReplicaSet", owner.Kind, "all control-plane writers must stop before fixture seeding")
			}
		}
	}, cleanupCommandTimeout, jobPollInterval)
	return restore
}

func waitHashTrackingIdle(t *testing.T, ctx context.Context, client runtimeclient.Client, owned map[string]bool) {
	t.Helper()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		all := &ucpv1alpha1.ResourceList{}
		if !assert.NoError(c, client.List(ctx, all, runtimeclient.InNamespace(radiusNamespace))) {
			return
		}
		matches := 0
		for _, resource := range all.Items {
			for _, entry := range resource.Entries {
				id, err := resources.Parse(entry.ID)
				if !assert.NoError(c, err) || !strings.EqualFold(id.Type(), datamodel.GenericResourceType) {
					continue
				}
				var tracked datamodel.GenericResource
				if !assert.NoError(c, json.Unmarshal(entry.Data.Raw, &tracked)) || !owned[strings.ToLower(tracked.Properties.ID)] {
					continue
				}
				matches++
				assert.True(c, tracked.AsyncProvisioningState.IsTerminal())
			}
		}
		assert.Positive(c, matches)
	}, cleanupTimeout, jobPollInterval)
}

var hashFixtureNamePattern = regexp.MustCompile(`^[a-z0-9-]+$`)

func hashFixtureObjectName(id resources.ID, legacy bool) (string, error) {
	name := strings.ToLower(id.Name())
	if !id.IsResource() || !hashFixtureNamePattern.MatchString(name) {
		return "", fmt.Errorf("unsupported fixture resource ID %q", id.String())
	}
	input := []byte(strings.ToLower(id.String()))
	digest := hashutil.Hex(input)[:40]
	if legacy {
		digest = hashutil.LegacyHex(input)
	}
	return "resource." + name + "." + digest, nil
}

// These fixture encodings follow the code before 862fc3d5a (#12255). This is test-only
// data preparation, not an online re-key migration or a claim about old-binary rollouts.
func legacyHashFixture(source *ucpv1alpha1.Resource, owned map[string]bool) (*ucpv1alpha1.Resource, error) {
	result := source.DeepCopy()
	selected := false
	for index := range result.Entries {
		entry := &result.Entries[index]
		id, err := resources.Parse(entry.ID)
		if err != nil {
			return nil, err
		}
		if entry.Data == nil && (owned[strings.ToLower(entry.ID)] || strings.EqualFold(id.Type(), datamodel.GenericResourceType)) {
			return nil, fmt.Errorf("fixture resource %q has no data", entry.ID)
		}
		if strings.EqualFold(id.Type(), datamodel.GenericResourceType) {
			var tracked datamodel.GenericResource
			if err := json.Unmarshal(entry.Data.Raw, &tracked); err != nil {
				return nil, err
			}
			if !owned[strings.ToLower(tracked.Properties.ID)] {
				continue
			}
			original, err := resources.Parse(tracked.Properties.ID)
			if err != nil {
				return nil, err
			}
			id = trackedresource.LegacyIDFor(original)
			var data map[string]json.RawMessage
			if err := json.Unmarshal(entry.Data.Raw, &data); err != nil {
				return nil, err
			}
			data["id"], err = json.Marshal(id.String())
			if err != nil {
				return nil, err
			}
			data["name"], err = json.Marshal(id.Name())
			if err != nil {
				return nil, err
			}
			entry.Data.Raw, err = json.Marshal(data)
			if err != nil {
				return nil, err
			}
		} else if !owned[strings.ToLower(entry.ID)] {
			continue
		}
		if len(result.Entries) != 1 {
			return nil, fmt.Errorf("fixture refuses to rewrite a shared bucket %q", source.Name)
		}
		result.Name, err = hashFixtureObjectName(id, true)
		if err != nil {
			return nil, err
		}
		entry.ID = id.String()
		entry.ETag = "20-" + hashutil.LegacyHex(entry.Data.Raw)
		selected = true
	}
	if !selected {
		return nil, nil
	}
	result.UID, result.ResourceVersion = "", ""
	result.CreationTimestamp = metav1.Time{}
	result.ManagedFields = nil
	result.Generation = 0
	return result, nil
}

func seedHashIdentities(t *testing.T, ctx context.Context, client runtimeclient.Client, owned map[string]bool) map[string]types.UID {
	t.Helper()
	all := &ucpv1alpha1.ResourceList{}
	require.NoError(t, client.List(ctx, all, runtimeclient.InNamespace(radiusNamespace)))
	identities := map[string]types.UID{}
	for index := range all.Items {
		original := &all.Items[index]
		legacy, err := legacyHashFixture(original, owned)
		require.NoError(t, err)
		if legacy == nil {
			continue
		}
		require.NotEqual(t, original.Name, legacy.Name)
		require.NoError(t, client.Create(ctx, legacy))
		require.NoError(t, client.Delete(ctx, original, &runtimeclient.DeleteOptions{Preconditions: &metav1.Preconditions{
			UID: &original.UID, ResourceVersion: &original.ResourceVersion,
		}}))
		identities[legacy.Name] = legacy.UID
	}
	require.GreaterOrEqual(t, len(identities), len(owned)+1, "seed both resource and tracking identity layers")
	return identities
}

// waitHashMigration waits until every component records that its SHA-256 migration is done, and returns the statuses.
func waitHashMigration(t *testing.T, ctx context.Context, kube kubernetes.Interface) map[string]hashmigration.Status {
	t.Helper()
	var statuses map[string]hashmigration.Status
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		statuses, err = hashmigration.ReadStatus(ctx, kube, radiusNamespace)
		if !assert.NoError(c, err) {
			return
		}
		for _, key := range hashMigrationKeys {
			status, ok := statuses[key]
			if assert.True(c, ok, "missing migration status %s", key) {
				assert.True(c, status.Done(), "migration %s: %+v", key, status)
				assert.Zero(c, status.Conflicts, "migration %s must not find conflicting copies", key)
			}
		}
	}, hashMigrationTimeout, jobPollInterval)
	return statuses
}

// requireHashMigrated checks that every seeded legacy identity was moved to its current name exactly once.
func requireHashMigrated(t *testing.T, ctx context.Context, client runtimeclient.Client, legacyObjects map[string]types.UID, owned map[string]bool) {
	t.Helper()
	for name := range legacyObjects {
		err := client.Get(ctx, runtimeclient.ObjectKey{Namespace: radiusNamespace, Name: name}, &ucpv1alpha1.Resource{})
		require.True(t, apierrors.IsNotFound(err), "legacy object %s must be migrated", name)
	}

	all := &ucpv1alpha1.ResourceList{}
	require.NoError(t, client.List(ctx, all, runtimeclient.InNamespace(radiusNamespace)))
	locations := map[string][]string{}
	tracking := map[string][]string{}
	for _, resource := range all.Items {
		for _, entry := range resource.Entries {
			key := strings.ToLower(entry.ID)
			locations[key] = append(locations[key], resource.Name)
			id, err := resources.Parse(entry.ID)
			require.NoError(t, err)
			if strings.EqualFold(id.Type(), datamodel.GenericResourceType) {
				var tracked datamodel.GenericResource
				require.NoError(t, json.Unmarshal(entry.Data.Raw, &tracked))
				require.Equal(t, entry.ID, tracked.ID)
				original := strings.ToLower(tracked.Properties.ID)
				tracking[original] = append(tracking[original], key)
			}
		}
	}

	trackedCount := 0
	for original := range owned {
		id := resources.MustParse(original)
		name, err := hashFixtureObjectName(id, false)
		require.NoError(t, err)
		require.Equal(t, []string{name}, locations[original], "%s must be stored once, under its current name", original)

		if len(tracking[original]) == 0 {
			continue
		}
		trackedCount++
		trackingID := trackedresource.IDFor(id)
		require.Equal(t, []string{strings.ToLower(trackingID.String())}, tracking[original], "%s must be tracked once, under its current tracking ID", original)
		trackingName, err := hashFixtureObjectName(trackingID, false)
		require.NoError(t, err)
		require.Equal(t, []string{trackingName}, locations[strings.ToLower(trackingID.String())])
	}
	require.Positive(t, trackedCount, "the fixture must migrate tracking identities")
}

// requireHashStateMoved checks that the migration moved the Terraform state from its legacy to its current Secret,
// keeping the state and holding the legacy lock, and returns the moved state.
func requireHashStateMoved(t *testing.T, ctx context.Context, kube kubernetes.Interface, before hashState, legacy string, current string) hashState {
	t.Helper()
	moved := readHashState(t, ctx, kube, current)
	require.Equal(t, before.Lineage, moved.Lineage)
	require.Equal(t, before.ID, moved.ID, "Terraform must not recreate the managed resource")
	require.Equal(t, before.Input, moved.Input)
	require.GreaterOrEqual(t, moved.Serial, before.Serial)
	require.NotEqual(t, before.UID, moved.UID, "the state must be moved to its current Secret")

	secret, err := kube.CoreV1().Secrets(radiusNamespace).Get(ctx, current, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, legacy, secret.Annotations[backends.AnnotationStateMigratedFrom])
	require.Equal(t, string(before.UID), secret.Annotations[backends.AnnotationStateMigratedFromUID])

	_, err = kube.CoreV1().Secrets(radiusNamespace).Get(ctx, legacy, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err), "the legacy Terraform state must be removed")
	lease, err := kube.CoordinationV1().Leases(radiusNamespace).Get(ctx, "lock-"+legacy, metav1.GetOptions{})
	require.NoError(t, err)
	require.NotNil(t, lease.Spec.HolderIdentity)
	require.Equal(t, backends.StateMigrationTombstone, *lease.Spec.HolderIdentity, "the legacy lock must stay held")
	return moved
}

// requireHashSecretHashes checks that a Pod template that still carries the legacy hash of its unchanged secret kept
// it, and that the migration recorded the current hash of that secret on the Deployment.
func requireHashSecretHashes(t *testing.T, ctx context.Context, kube kubernetes.Interface) {
	t.Helper()
	deployments, err := kube.AppsV1().Deployments(hashApplication).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	checked := 0
	for _, deployment := range deployments.Items {
		hash := deployment.Spec.Template.Annotations[radiuskubernetes.AnnotationSecretHash]
		if hash == "" {
			continue
		}
		checked++
		if !radiuskubernetes.IsLegacySecretHash(hash) {
			require.True(t, strings.HasPrefix(hash, radiuskubernetes.SecretHashPrefix), "unexpected secret hash %q", hash)
			continue
		}
		secret, err := kube.CoreV1().Secrets(deployment.Namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, radiuskubernetes.LegacyHashSecretData(secret.Data), hash, "the Pod template must keep the hash of its unchanged secret")
		require.Equal(t, radiuskubernetes.HashSecretData(secret.Data), deployment.Annotations[radiuskubernetes.AnnotationSecretHashSHA256])
	}
	require.Positive(t, checked, "the fixture must exercise the secret-hash pod annotation")
}
func hashStateSecret(id resources.ID, legacy bool) string {
	input := []byte(strings.ToLower(hashEnvironment + "-" + hashApplication + "-" + id.String()))
	digest := hashutil.Hex(input)[:40]
	if legacy {
		digest = hashutil.LegacyHex(input)
	}
	return backends.KubernetesBackendNamePrefix + digest
}

func seedHashState(t *testing.T, ctx context.Context, kube kubernetes.Interface, current, legacy string) {
	t.Helper()
	secret, err := kube.CoreV1().Secrets(radiusNamespace).Get(ctx, current, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "true", secret.Labels["tfstate"])
	require.Equal(t, strings.TrimPrefix(current, backends.KubernetesBackendNamePrefix), secret.Labels["tfstateSecretSuffix"])
	lease, err := kube.CoordinationV1().Leases(radiusNamespace).Get(ctx, "lock-"+current, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		lease = nil
	} else {
		require.NoError(t, err)
		require.Nil(t, lease.Spec.HolderIdentity, "the fixture must not move actively locked state")
	}
	copy := secret.DeepCopy()
	copy.Name, copy.UID, copy.ResourceVersion = legacy, "", ""
	copy.CreationTimestamp = metav1.Time{}
	copy.ManagedFields = nil
	copy.Labels["tfstateSecretSuffix"] = strings.TrimPrefix(legacy, backends.KubernetesBackendNamePrefix)
	_, err = kube.CoreV1().Secrets(radiusNamespace).Create(ctx, copy, metav1.CreateOptions{})
	require.NoError(t, err)
	require.NoError(t, kube.CoreV1().Secrets(radiusNamespace).Delete(ctx, current, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &secret.UID, ResourceVersion: &secret.ResourceVersion},
	}))
	if lease == nil {
		return
	}
	require.NoError(t, kube.CoordinationV1().Leases(radiusNamespace).Delete(ctx, lease.Name, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &lease.UID, ResourceVersion: &lease.ResourceVersion},
	}))
}

type hashState struct {
	Lineage string
	Serial  int64
	ID      string
	UID     types.UID
	Input   string
}

func readHashState(t *testing.T, ctx context.Context, kube kubernetes.Interface, name string) hashState {
	t.Helper()
	secret, err := kube.CoreV1().Secrets(radiusNamespace).Get(ctx, name, metav1.GetOptions{})
	require.NoError(t, err)
	reader, err := gzip.NewReader(bytes.NewReader(secret.Data["tfstate"]))
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	var state struct {
		Lineage   string `json:"lineage"`
		Serial    int64  `json:"serial"`
		Resources []struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Instances []struct {
				Attributes struct {
					ID    string `json:"id"`
					Input struct {
						Value string `json:"value"`
					} `json:"input"`
				} `json:"attributes"`
			} `json:"instances"`
		} `json:"resources"`
	}
	require.NoError(t, json.Unmarshal(data, &state))
	result := hashState{Lineage: state.Lineage, Serial: state.Serial, UID: secret.UID}
	for _, resource := range state.Resources {
		if resource.Type == "terraform_data" && resource.Name == "identity" {
			require.Len(t, resource.Instances, 1)
			result.ID = resource.Instances[0].Attributes.ID
			result.Input = resource.Instances[0].Attributes.Input.Value
		}
	}
	require.NotEmpty(t, result.Lineage)
	require.NotEmpty(t, result.ID)
	return result
}

func requireHashState(t require.TestingT, before, after hashState, expectedInput string) {
	if helper, ok := t.(interface{ Helper() }); ok {
		helper.Helper()
	}
	require.Equal(t, before.Lineage, after.Lineage)
	require.Equal(t, before.ID, after.ID, "Terraform must not recreate the managed resource")
	require.Equal(t, before.UID, after.UID, "Terraform must preserve the selected state secret")
	require.GreaterOrEqual(t, after.Serial, before.Serial)
	require.Equal(t, expectedInput, after.Input, "Terraform must persist the requested input")
}

func Test_HashStateInput(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		inputJSON     string
		expectedInput string
		valid         bool
	}{
		{name: "updated", inputJSON: `{"value":"updated","type":"string"}`, expectedInput: "updated", valid: true},
		{name: "unchanged_no_op", inputJSON: `{"value":"before","type":"string"}`, expectedInput: "before", valid: true},
		{name: "unchanged_despite_advanced_serial", inputJSON: `{"value":"before","type":"string"}`, expectedInput: "updated"},
		{name: "incorrect", inputJSON: `{"value":"incorrect","type":"string"}`, expectedInput: "updated"},
		{name: "empty", inputJSON: `{"value":"","type":"string"}`, expectedInput: "updated"},
		{name: "missing_value", inputJSON: `{"type":"string"}`, expectedInput: "updated"},
		{name: "null", inputJSON: `null`, expectedInput: "updated"},
		{name: "missing", expectedInput: "updated"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := ""
			if tt.inputJSON != "" {
				input = `"input":` + tt.inputJSON + `,`
			}
			data := fmt.Sprintf(`{
				"version": 4,
				"lineage": "state-lineage",
				"serial": 11,
				"resources": [{
					"type": "terraform_data",
					"name": "identity",
					"instances": [{"attributes": {%s"id": "resource-id"}}]
				}]
			}`, input)
			var compressed bytes.Buffer
			writer := gzip.NewWriter(&compressed)
			_, err := writer.Write([]byte(data))
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "legacy-state", Namespace: radiusNamespace, UID: "state-uid"},
				Data:       map[string][]byte{"tfstate": compressed.Bytes()},
			}
			kube := kubernetesfake.NewClientset(secret)
			state := readHashState(t, t.Context(), kube, secret.Name)
			before := hashState{Lineage: "state-lineage", Serial: 10, ID: "resource-id", UID: secret.UID, Input: "before"}

			// CollectT exits the goroutine on a failed requirement; observe whether all assertions complete.
			completed := make(chan bool, 1)
			go func() {
				passed := false
				defer func() { completed <- passed }()
				requireHashState(&assert.CollectT{}, before, state, tt.expectedInput)
				passed = true
			}()
			require.Equal(t, tt.valid, <-completed)
		})
	}
}

func writeAcrossHashUpgrade(ctx context.Context, cli *radcli.CLI, parameters []string, upgrade func() error) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	stop := make(chan struct{})
	type result struct {
		successes int
		err       error
	}
	writes := make(chan result, 1)
	go func() {
		state := result{}
		for {
			err := cli.Deploy(ctx, hashTemplate, "", "", parameters...)
			if err == nil {
				state.successes++
				select {
				case <-stop:
					writes <- state
					return
				default:
				}
			} else if !step.IsTransientDeployError(err) {
				state.err = err
				writes <- state
				return
			}
			select {
			case <-ctx.Done():
				state.err = ctx.Err()
				writes <- state
				return
			case <-time.After(time.Second):
			}
		}
	}()
	err := upgrade()
	close(stop)
	if err != nil {
		cancel()
	}
	outcome := <-writes
	return outcome.successes, errors.Join(err, outcome.err)
}

func Test_HashFixtureIdentity(t *testing.T) {
	t.Parallel()
	id := resources.MustParse("/planes/radius/local/resourceGroups/test/providers/Applications.Core/applications/target")
	current, err := hashFixtureObjectName(id, false)
	require.NoError(t, err)
	require.Equal(t, "resource.target.fcb5eea8e5786641962523c28a0ae5ca666f04ad", current)
	legacy, err := hashFixtureObjectName(id, true)
	require.NoError(t, err)
	require.Equal(t, "resource.target.e62476ba4a2e30f885832553c32369f34566a2e6", legacy)
	owned := map[string]bool{strings.ToLower(id.String()): true}
	original := &ucpv1alpha1.Resource{
		Name: current, Namespace: radiusNamespace, UID: "original-uid", ResourceVersion: "7",
		Entries: []ucpv1alpha1.ResourceEntry{{ID: id.String(), Data: &runtime.RawExtension{Raw: []byte(`{"properties":{"value":"keep"}}`)}}},
	}
	converted, err := legacyHashFixture(original, owned)
	require.NoError(t, err)
	require.Equal(t, legacy, converted.Name)
	require.Empty(t, converted.UID)
	require.Empty(t, converted.ResourceVersion)
	require.Equal(t, original.Entries[0].Data.Raw, converted.Entries[0].Data.Raw)
	require.Equal(t, current, original.Name)
	tracked := datamodel.GenericResourceFromID(id, trackedresource.IDFor(id))
	data, err := json.Marshal(tracked)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &fields))
	fields["additionalMetadata"] = json.RawMessage(`{"preserve":true}`)
	data, err = json.Marshal(fields)
	require.NoError(t, err)
	source := &ucpv1alpha1.Resource{Name: "current", Namespace: radiusNamespace, Entries: []ucpv1alpha1.ResourceEntry{
		{ID: tracked.ID, Data: &runtime.RawExtension{Raw: data}},
	}}
	fixture, err := legacyHashFixture(source, owned)
	require.NoError(t, err)
	require.NotNil(t, fixture)
	expected := trackedresource.LegacyIDFor(id)
	require.Equal(t, expected.String(), fixture.Entries[0].ID)
	var got datamodel.GenericResource
	require.NoError(t, json.Unmarshal(fixture.Entries[0].Data.Raw, &got))
	require.Equal(t, expected.String(), got.ID)
	require.Equal(t, expected.Name(), got.Name)
	require.Equal(t, id.String(), got.Properties.ID)
	require.NoError(t, json.Unmarshal(fixture.Entries[0].Data.Raw, &fields))
	require.JSONEq(t, `{"preserve":true}`, string(fields["additionalMetadata"]))
	require.Regexp(t, `^20-[0-9a-f]{40}$`, fixture.Entries[0].ETag)
	require.Equal(t, tracked.ID, source.Entries[0].ID)
	ignored, err := legacyHashFixture(source, map[string]bool{})
	require.NoError(t, err)
	require.Nil(t, ignored)
	source.Entries = append(source.Entries, source.Entries[0])
	_, err = legacyHashFixture(source, owned)
	require.ErrorContains(t, err, "shared bucket")
	source.Entries = source.Entries[:1]
	source.Entries[0].Data = nil
	_, err = legacyHashFixture(source, owned)
	require.ErrorContains(t, err, "has no data")
}
