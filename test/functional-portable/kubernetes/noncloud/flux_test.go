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

package kubernetes_noncloud_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/controller/reconciler"
	"github.com/radius-project/radius/test/rp"
	"go.yaml.in/yaml/v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	gitobject "github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/remotecommand"
	watchtools "k8s.io/client-go/tools/watch"
	"k8s.io/client-go/util/retry"
)

const (
	fluxSystemNamespace             = "flux-system"
	testGitServerURLEnvVariableName = "GIT_HTTP_SERVER_URL"
	testGitUsernameEnvVariableName  = "GIT_HTTP_USERNAME"
	testGitEmailEnvVariableName     = "GIT_HTTP_EMAIL"
	testGitPasswordEnvVariableName  = "GIT_HTTP_PASSWORD"

	gitServerNamespace             = "git-http-backend"
	gitServerLabelSelector         = "app=git-http-backend"
	gitServerContainerName         = "git-http-backend"
	gitServerInternalRepoURLFormat = "http://git-http.git-http-backend.svc.cluster.local:3000/%s.git"

	// fluxGitRepositoryInterval is the periodic reconcile interval for the Flux
	// GitRepository. It is a backstop in case a reconcile request is missed.
	fluxGitRepositoryInterval = 30 * time.Second
	// fluxArtifactTimeout is how long to wait for source-controller to produce
	// an artifact for a pushed commit.
	fluxArtifactTimeout = 5 * time.Minute
	// fluxDeploymentTemplateTimeout is how long to wait for a DeploymentTemplate
	// to become Ready or to be pruned by the Radius Flux controller.
	fluxDeploymentTemplateTimeout = 10 * time.Minute
	// fluxResourceAssertionTimeout is how long to wait for the expected Radius
	// resources to converge after a step has been applied.
	fluxResourceAssertionTimeout = 5 * time.Minute
	fluxPollInterval             = 2 * time.Second
)

func Test_Flux_Basic(t *testing.T) {
	testName := "flux-basic"
	steps := []GitOpsTestStep{
		{
			path:          "testdata/gitops/basic/step1",
			resourceGroup: "flux-basic",
			expectedResources: [][]string{
				{"Applications.Core/environments", "flux-basic-env"},
			},
		},
		{
			path:          "testdata/gitops/basic/step2",
			resourceGroup: "flux-basic",
			expectedResourcesToNotExist: [][]string{
				{"Applications.Core/environments", "flux-basic-env"},
			},
		},
	}

	namespaces := []string{
		"flux-basic",
	}

	testFluxIntegration(t, testName, steps, namespaces, false)
}

func Test_Flux_Complex(t *testing.T) {
	testName := "flux-complex"
	steps := []GitOpsTestStep{
		{
			path:          "testdata/gitops/complex/step1",
			resourceGroup: "flux-complex",
			expectedResources: [][]string{
				{"Applications.Core/environments", "flux-complex-env"},
				{"Applications.Core/applications", "flux-complex-app"},
				{"Applications.Core/containers", "flux-complex-container"},
			},
		},
		{
			path:          "testdata/gitops/complex/step2",
			resourceGroup: "flux-complex",
			expectedResources: [][]string{
				{"Applications.Core/environments", "flux-complex-env"},
				{"Applications.Core/applications", "flux-complex-app"},
				{"Applications.Core/containers", "flux-complex-container-2"},
			},
		},
		{
			path:          "testdata/gitops/complex/step3",
			resourceGroup: "flux-complex",
			expectedResourcesToNotExist: [][]string{
				{"Applications.Core/environments", "flux-complex-env"},
				{"Applications.Core/applications", "flux-complex-app"},
				{"Applications.Core/containers", "flux-complex-container"},
				{"Applications.Core/containers", "flux-complex-container-2"},
			},
		},
	}

	namespaces := []string{
		"flux-complex",
		"flux-complex-flux-complex-app",
	}

	testFluxIntegration(t, testName, steps, namespaces, false)
}

func Test_Flux_SourceSelection(t *testing.T) {
	steps := []GitOpsTestStep{
		{
			path: "testdata/gitops/basic/step1", resourceGroup: "flux-basic",
			expectedResources: [][]string{{"Applications.Core/environments", "flux-basic-env"}},
		},
		{
			path: "testdata/gitops/basic/step2", resourceGroup: "flux-basic",
			expectedResourcesToNotExist: [][]string{{"Applications.Core/environments", "flux-basic-env"}},
		},
	}
	testFluxIntegration(t, "flux-selection", steps, []string{"flux-basic"}, true)
}

type fluxSelectionFixture struct {
	client          kubernetes.Interface
	originalConfig  string
	originalRestart string
	hadRestart      bool
}

const fluxRestartAnnotation = "radapp.io/flux-test-restart"

func newFluxSelectionFixture(t *testing.T, ctx context.Context, client kubernetes.Interface) *fluxSelectionFixture {
	t.Helper()
	cm, err := client.CoreV1().ConfigMaps("radius-system").Get(ctx, "controller-config", metav1.GetOptions{})
	require.NoError(t, err)
	deployment, err := client.AppsV1().Deployments("radius-system").Get(ctx, "controller", metav1.GetOptions{})
	require.NoError(t, err)
	value, present := deployment.Spec.Template.Annotations[fluxRestartAnnotation]
	fixture := &fluxSelectionFixture{client: client, originalConfig: cm.Data["controller-config.yaml"], originalRestart: value, hadRestart: present}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 4*time.Minute)
		defer cancel()
		require.NoError(t, fixture.apply(cleanupCtx, fixture.originalConfig, fixture.originalRestart, fixture.hadRestart))
	})
	return fixture
}

func (f *fluxSelectionFixture) selectSources(t *testing.T, ctx context.Context, sources []hostoptions.FluxSourceSelection) {
	t.Helper()
	cm, err := f.client.CoreV1().ConfigMaps("radius-system").Get(ctx, "controller-config", metav1.GetOptions{})
	require.NoError(t, err)
	var config map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(cm.Data["controller-config.yaml"]), &config))
	config["controller"] = hostoptions.ControllerOptions{Flux: hostoptions.FluxOptions{Enabled: true, Sources: sources}}
	data, err := yaml.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, f.apply(ctx, string(data), time.Now().Format(time.RFC3339Nano), true))
}

func (f *fluxSelectionFixture) apply(ctx context.Context, config, restart string, includeRestart bool) error {
	oldPods, err := f.client.CoreV1().Pods("radius-system").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=controller"})
	if err != nil {
		return err
	}
	oldUIDs := make(map[types.UID]bool, len(oldPods.Items))
	for _, pod := range oldPods.Items {
		oldUIDs[pod.UID] = true
	}
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		cm, err := f.client.CoreV1().ConfigMaps("radius-system").Get(ctx, "controller-config", metav1.GetOptions{})
		if err != nil {
			return err
		}
		cm.Data["controller-config.yaml"] = config
		_, err = f.client.CoreV1().ConfigMaps("radius-system").Update(ctx, cm, metav1.UpdateOptions{})
		return err
	}); err != nil {
		return err
	}
	var desiredGeneration int64
	var restarted bool
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		deployment, err := f.client.AppsV1().Deployments("radius-system").Get(ctx, "controller", metav1.GetOptions{})
		if err != nil {
			return err
		}
		if deployment.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType {
			return fmt.Errorf("Flux fixture requires the controller Recreate strategy")
		}
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = map[string]string{}
		}
		if includeRestart {
			deployment.Spec.Template.Annotations[fluxRestartAnnotation] = restart
		} else {
			delete(deployment.Spec.Template.Annotations, fluxRestartAnnotation)
		}
		updated, err := f.client.AppsV1().Deployments("radius-system").Update(ctx, deployment, metav1.UpdateOptions{})
		if err == nil {
			desiredGeneration = updated.Generation
			restarted = updated.Generation != deployment.Generation
		}
		return err
	}); err != nil {
		return err
	}
	if !restarted {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	return wait.PollUntilContextCancel(waitCtx, time.Second, true, func(ctx context.Context) (bool, error) {
		deployment, err := f.client.AppsV1().Deployments("radius-system").Get(ctx, "controller", metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if deployment.Status.ObservedGeneration < desiredGeneration || deployment.Status.UpdatedReplicas != 1 ||
			deployment.Status.ReadyReplicas != 1 || deployment.Status.AvailableReplicas != 1 {
			return false, nil
		}
		pods, err := f.client.CoreV1().Pods("radius-system").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=controller"})
		if err != nil {
			return false, err
		}
		for _, pod := range pods.Items {
			if oldUIDs[pod.UID] {
				return false, nil
			}
		}
		return len(pods.Items) == 1, nil
	})
}

// testFluxIntegration is a helper function that runs a test for the integration of Radius and Flux.
func testFluxIntegration(t *testing.T, testName string, steps []GitOpsTestStep, namespaces []string, checkSelection bool) {
	ctx := t.Context()
	opts := rp.NewRPTestOptions(t)

	gitRepoName := fmt.Sprintf("%s-repo", testName)
	gitServerURL := os.Getenv(testGitServerURLEnvVariableName)
	gitUsername := os.Getenv(testGitUsernameEnvVariableName)
	gitEmail := os.Getenv(testGitEmailEnvVariableName)
	gitPassword := os.Getenv(testGitPasswordEnvVariableName)

	require.NotEmpty(t, gitServerURL, fmt.Sprintf("%s must be set", testGitServerURLEnvVariableName))
	require.NotEmpty(t, gitUsername, fmt.Sprintf("%s must be set", testGitUsernameEnvVariableName))
	require.NotEmpty(t, gitEmail, fmt.Sprintf("%s must be set", testGitEmailEnvVariableName))
	require.NotEmpty(t, gitPassword, fmt.Sprintf("%s must be set", testGitPasswordEnvVariableName))

	cleanupGitRepo := ensureGitHTTPRepository(ctx, t, opts, gitRepoName)
	defer cleanupGitRepo()

	// Create a temp directory for the Git repository.
	dir, err := os.MkdirTemp("", gitRepoName)
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// Initialize a new Git repository in the temp directory.
	r, err := git.PlainInit(dir, false)
	require.NoError(t, err)

	// Get the worktree of the repository.
	w, err := r.Worktree()
	require.NoError(t, err)

	// Create the remote for the repository.
	trimmedGitServerURL := strings.TrimSuffix(gitServerURL, "/")
	repoURL := fmt.Sprintf("%s/%s.git", trimmedGitServerURL, gitRepoName)
	t.Logf("Using git HTTP repository URL: %s", repoURL)
	_, err = r.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{repoURL},
	})
	require.NoError(t, err)

	waitForGitHTTPRepository(t, repoURL, gitUsername, gitPassword)

	// Create an initial commit to the repository.
	commit, err := w.Commit("Initial commit", &git.CommitOptions{
		AllowEmptyCommits: true,
		Author: &gitobject.Signature{
			Name:  gitUsername,
			Email: gitEmail,
			When:  time.Now(),
		},
	})
	require.NoError(t, err)
	t.Log(t, "Commit created:", commit)

	// Push the initial commit back to the repository.
	err = r.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth: &githttp.BasicAuth{
			Username: gitUsername,
			Password: gitPassword,
		},
	})
	require.NoError(t, err)
	t.Log(t, "Pushed changes successfully")

	// Create the secret for the Flux GitRepository.
	secret := &corev1.Secret{
		Name:      gitRepoName,
		Namespace: fluxSystemNamespace,
		Type:      corev1.SecretTypeBasicAuth,
		Data: map[string][]byte{
			"username": []byte(gitUsername),
			"password": []byte(gitPassword),
		},
	}
	err = opts.Client.Create(ctx, secret)
	defer func() {
		err := opts.Client.Delete(ctx, secret)
		require.NoError(t, err)
	}()
	require.NoError(t, err)

	// Create the Flux GitRepository object.
	fluxGitRepository := &sourcev1.GitRepository{
		Name:      gitRepoName,
		Namespace: fluxSystemNamespace,
		Spec: sourcev1.GitRepositorySpec{
			URL:      fmt.Sprintf(gitServerInternalRepoURLFormat, gitRepoName),
			Interval: metav1.Duration{Duration: fluxGitRepositoryInterval},
			SecretRef: &meta.LocalObjectReference{
				Name: gitRepoName,
			},
		},
	}
	err = opts.Client.Create(ctx, fluxGitRepository)
	defer func() {
		err := opts.Client.Delete(ctx, fluxGitRepository)
		require.NoError(t, err)
	}()
	require.NoError(t, err)

	// Wait for the GitRepository to be ready.
	_, err = waitForGitRepositoryReady(t, ctx, types.NamespacedName{Name: gitRepoName, Namespace: fluxSystemNamespace}, opts.Client, fluxGitRepository.ResourceVersion)
	require.NoError(t, err)
	selection := []hostoptions.FluxSourceSelection{{
		Namespace: fluxGitRepository.Namespace, Name: fluxGitRepository.Name, UID: string(fluxGitRepository.UID),
	}}
	fixture := newFluxSelectionFixture(t, ctx, opts.K8sClient)
	if checkSelection {
		fixture.selectSources(t, ctx, []hostoptions.FluxSourceSelection{})
	} else {
		fixture.selectSources(t, ctx, selection)
	}

	cleanupDTs := map[types.NamespacedName]struct{}{}
	for stepIndex, step := range steps {
		stepNumber := stepIndex + 1
		gitRepoNN := types.NamespacedName{Name: gitRepoName, Namespace: fluxSystemNamespace}
		if checkSelection && stepIndex > 0 {
			// Keep the old artifact inactive until the next configuration is pushed.
			err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				repo := &sourcev1.GitRepository{}
				if err := opts.Client.Get(ctx, gitRepoNN, repo); err != nil {
					return err
				}
				repo.Spec.Suspend = true
				return opts.Client.Update(ctx, repo)
			})
			require.NoError(t, err)
			var suspended sourcev1.GitRepository
			require.NoError(t, opts.Client.Get(ctx, gitRepoNN, &suspended))
			require.Equal(t, fluxGitRepository.UID, suspended.UID)
			require.Greater(t, suspended.Generation, fluxGitRepository.Generation)
		}

		// Remove all files from the repository.
		// Add all files from step.path to the repository.
		err = addFilesToRepository(w, step.path, dir)
		require.NoError(t, err)

		// Commit the change.
		commit, err := w.Commit(fmt.Sprintf("Adding files to %s from step %d", gitRepoName, stepNumber), &git.CommitOptions{
			Author: &gitobject.Signature{
				Name:  gitUsername,
				Email: gitEmail,
				When:  time.Now(),
			},
		})
		require.NoError(t, err)
		t.Log(t, "Commit created:", commit)

		// Push the commit back to the repository.
		err = r.Push(&git.PushOptions{
			RemoteName: "origin",
			Auth: &githttp.BasicAuth{
				Username: gitUsername,
				Password: gitPassword,
			},
		})
		require.NoError(t, err)
		t.Log("Pushed changes successfully")

		// Request an immediate reconcile of the GitRepository. RFC3339Nano is used so
		// that two requests issued within the same second are still distinct;
		// otherwise source-controller treats the second request as already handled.
		err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			repo := &sourcev1.GitRepository{}
			if err := opts.Client.Get(ctx, gitRepoNN, repo); err != nil {
				return err
			}
			annotations := repo.GetAnnotations()
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations[meta.ReconcileRequestAnnotation] = time.Now().Format(time.RFC3339Nano)
			repo.SetAnnotations(annotations)
			if checkSelection && stepIndex > 0 {
				repo.Spec.Suspend = false
			}
			return opts.Client.Update(ctx, repo)
		})
		require.NoError(t, err)

		// Wait for source-controller to publish an artifact for this exact commit
		// before checking anything else. Without this, later steps can be pushed
		// before earlier ones are reconciled and intermediate states are skipped.
		waitForGitRepositoryArtifactRevision(ctx, t, gitRepoNN, commit.String(), opts.Client)

		radiusConfig, err := reconciler.ParseRadiusGitOpsConfig(path.Join(step.path, "radius-gitops-config.yaml"))
		require.NoError(t, err)
		if checkSelection && stepIndex == 0 {
			notSelectedErr := wait.PollUntilContextTimeout(ctx, time.Second, 10*time.Second, true, func(_ context.Context) (bool, error) {
				// Give the final read its own budget rather than the polling window's remaining milliseconds.
				readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				for _, entry := range radiusConfig.Config {
					name, namespace, _, _ := getValuesFromRadiusGitOpsConfig(entry)
					err := opts.Client.Get(readCtx, types.NamespacedName{Namespace: namespace, Name: name}, &radappiov1alpha3.DeploymentTemplate{})
					if err == nil {
						return false, fmt.Errorf("unselected source created DeploymentTemplate %s/%s", namespace, name)
					}
					if !apierrors.IsNotFound(err) {
						return false, err
					}
				}
				return false, nil
			})
			require.ErrorIs(t, notSelectedErr, context.DeadlineExceeded, "unselected source must not create templates")
			fixture.selectSources(t, ctx, selection)
		}

		// DeploymentTemplates from earlier steps that are no longer in the config
		// must be pruned by the Radius Flux controller (not by the test).
		currentDTs := map[types.NamespacedName]struct{}{}
		for _, configEntry := range radiusConfig.Config {
			name, namespace, _, _ := getValuesFromRadiusGitOpsConfig(configEntry)
			nn := types.NamespacedName{Name: name, Namespace: namespace}
			currentDTs[nn] = struct{}{}
			if _, ok := cleanupDTs[nn]; !ok {
				cleanupDTs[nn] = struct{}{}
				// Best-effort cleanup if the test fails before the ordered teardown below.
				defer func() {
					dt := &radappiov1alpha3.DeploymentTemplate{Name: nn.Name, Namespace: nn.Namespace}
					if err := opts.Client.Delete(ctx, dt); controller_runtime.IgnoreNotFound(err) != nil {
						t.Logf("Error deleting deployment template %s: %v", nn, err)
					}
				}()
			}
		}
		for _, previousStep := range steps[:stepIndex] {
			previousConfig, err := reconciler.ParseRadiusGitOpsConfig(path.Join(previousStep.path, "radius-gitops-config.yaml"))
			require.NoError(t, err)
			for _, configEntry := range previousConfig.Config {
				name, namespace, _, _ := getValuesFromRadiusGitOpsConfig(configEntry)
				nn := types.NamespacedName{Name: name, Namespace: namespace}
				if _, ok := currentDTs[nn]; ok {
					continue
				}
				waitForDeploymentTemplateToBeDeleted(ctx, t, nn, opts.Client)
			}
		}

		scope := fmt.Sprintf("/planes/radius/local/resourceGroups/%s", step.resourceGroup)

		// Both checks must pass. Each returns nil for an empty list, so accepting
		// either one alone would let every step pass without checking anything.
		var lastAssertErr error
		assertOK := pollUntil(ctx, func() bool {
			if err := assertExpectedResourcesExist(ctx, scope, step.expectedResources, opts.Connection); err != nil {
				lastAssertErr = err
				return false
			}
			if err := assertExpectedResourcesToNotExist(ctx, scope, step.expectedResourcesToNotExist, opts.Connection); err != nil {
				lastAssertErr = err
				return false
			}
			lastAssertErr = nil
			return true
		}, fluxResourceAssertionTimeout)
		require.Truef(t, assertOK, "step %d: expected resources did not converge in %s: %v", stepNumber, scope, lastAssertErr)
		t.Logf("Step %d: successfully asserted expected resources in %s", stepNumber, scope)

		// Wait for every DeploymentTemplate in this step to settle before moving
		// on, so the next push does not race an in-flight deployment.
		for nn := range currentDTs {
			waitForDeploymentTemplateToBeReady(ctx, t, nn, opts.Client)
		}
		if checkSelection && stepIndex == 0 {
			before := map[types.NamespacedName]*radappiov1alpha3.DeploymentTemplate{}
			for nn := range currentDTs {
				dt := &radappiov1alpha3.DeploymentTemplate{}
				require.NoError(t, opts.Client.Get(ctx, nn, dt))
				require.NotNil(t, dt.Spec.SourceRef)
				require.Equal(t, fluxGitRepository.UID, dt.Spec.SourceRef.UID)
				before[nn] = dt
			}
			fixture.selectSources(t, ctx, []hostoptions.FluxSourceSelection{})
			for nn, original := range before {
				require.EventuallyWithT(t, func(check *assert.CollectT) {
					var dt radappiov1alpha3.DeploymentTemplate
					require.NoError(check, opts.Client.Get(ctx, nn, &dt))
					require.Equal(check, original.UID, dt.UID)
					require.Equal(check, original.Spec, dt.Spec)
					require.Equal(check, radappiov1alpha3.DeploymentTemplatePhraseHeld, dt.Status.Phrase)
				}, time.Minute, time.Second)
			}
			require.NoError(t, assertExpectedResourcesExist(ctx, scope, step.expectedResources, opts.Connection))
			fixture.selectSources(t, ctx, selection)
			for nn := range currentDTs {
				waitForDeploymentTemplateToBeReady(ctx, t, nn, opts.Client)
			}
		}
	}

	// Tear down any DeploymentTemplate that still exists (and wait for full
	// drainage of its owned DeploymentResources) before deleting the K8s
	// namespaces. DTs removed from the config were already pruned by Flux
	// during the steps above, so this only cleans up what remains. This
	// avoids racing parallel Radius delete cascades against the namespace
	// delete; without it, Test_Flux_Complex intermittently times out for
	// 10 minutes waiting for a stuck namespace.
	//
	// We union DTs across all steps so DTs created by earlier steps are
	// covered even if a later step failed before they were pruned.
	seen := map[types.NamespacedName]struct{}{}
	for _, step := range steps {
		radiusConfig, err := reconciler.ParseRadiusGitOpsConfig(path.Join(step.path, "radius-gitops-config.yaml"))
		require.NoError(t, err)
		for _, configEntry := range radiusConfig.Config {
			name, namespace, _, _ := getValuesFromRadiusGitOpsConfig(configEntry)
			nn := types.NamespacedName{Name: name, Namespace: namespace}
			if _, ok := seen[nn]; ok {
				continue
			}
			seen[nn] = struct{}{}
			t.Logf("Deleting DeploymentTemplate (and waiting): %s/%s", nn.Namespace, nn.Name)
			deleteDeploymentTemplateAndWait(ctx, t, nn, opts)
		}
	}

	for _, namespace := range namespaces {
		t.Logf("Deleting namespace: %s", namespace)
		deleteNamespace(ctx, t, namespace, opts)
	}
}

// pollUntil evaluates cond on the calling goroutine every fluxPollInterval
// until it returns true, the timeout elapses, or ctx is cancelled.
func pollUntil(ctx context.Context, cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(fluxPollInterval):
		}
	}
}

// waitForGitRepositoryArtifactRevision waits until the Flux GitRepository has
// produced an artifact for the given commit SHA. Flux reports revisions in the
// form "<branch>@sha1:<sha>".
func waitForGitRepositoryArtifactRevision(ctx context.Context, t *testing.T, nn types.NamespacedName, commitSHA string, client controller_runtime.Client) {
	t.Helper()
	lastRevision := ""
	ok := pollUntil(ctx, func() bool {
		repo := &sourcev1.GitRepository{}
		if err := client.Get(ctx, nn, repo); err != nil {
			t.Logf("Failed to get GitRepository %s: %v", nn, err)
			return false
		}
		if repo.Status.Artifact == nil {
			return false
		}
		if repo.Status.Artifact.Revision != lastRevision {
			lastRevision = repo.Status.Artifact.Revision
			t.Logf("GitRepository %s artifact revision: %s (waiting for %s)", nn, lastRevision, commitSHA)
		}
		return strings.HasSuffix(lastRevision, commitSHA)
	}, fluxArtifactTimeout)
	require.Truef(t, ok, "GitRepository %s did not produce an artifact for commit %s; last revision: %q", nn, commitSHA, lastRevision)
}

// waitForDeploymentTemplateToBeReady waits until the DeploymentTemplate exists,
// has no operation in flight, reports the Ready phrase, and its status reflects
// the current generation of its spec.
func waitForDeploymentTemplateToBeReady(ctx context.Context, t *testing.T, nn types.NamespacedName, client controller_runtime.Client) {
	t.Helper()
	lastState := "not observed"
	ok := pollUntil(ctx, func() bool {
		dt := &radappiov1alpha3.DeploymentTemplate{}
		if err := client.Get(ctx, nn, dt); err != nil {
			lastState = fmt.Sprintf("get error: %v", err)
			return false
		}
		state := fmt.Sprintf("phrase=%s generation=%d observedGeneration=%d operationInFlight=%t",
			dt.Status.Phrase, dt.Generation, dt.Status.ObservedGeneration, dt.Status.Operation != nil)
		if state != lastState {
			lastState = state
			t.Logf("DeploymentTemplate %s: %s", nn, state)
		}
		return dt.DeletionTimestamp == nil &&
			dt.Status.Phrase == radappiov1alpha3.DeploymentTemplatePhraseReady &&
			dt.Status.Operation == nil &&
			dt.Status.ObservedGeneration == dt.Generation
	}, fluxDeploymentTemplateTimeout)
	require.Truef(t, ok, "DeploymentTemplate %s did not become Ready; last state: %s", nn, lastState)
}

// waitForDeploymentTemplateToBeDeleted waits for the Radius Flux controller to
// prune a DeploymentTemplate that was removed from the GitOps config.
func waitForDeploymentTemplateToBeDeleted(ctx context.Context, t *testing.T, nn types.NamespacedName, client controller_runtime.Client) {
	t.Helper()
	t.Logf("Waiting for Flux to prune DeploymentTemplate %s", nn)
	var lastErr error
	ok := pollUntil(ctx, func() bool {
		err := client.Get(ctx, nn, &radappiov1alpha3.DeploymentTemplate{})
		if apierrors.IsNotFound(err) {
			lastErr = nil
			return true
		}
		lastErr = err
		return false
	}, fluxDeploymentTemplateTimeout)
	require.Truef(t, ok, "DeploymentTemplate %s was not pruned by Flux; last error: %v", nn, lastErr)
}

// waitForGitRepositoryReady watches the creation of the GitRepository object
// and waits for it to be in the "Ready" state.
func waitForGitRepositoryReady(t *testing.T, ctx context.Context, name types.NamespacedName, client controller_runtime.WithWatch, initialVersion string) (*sourcev1.GitRepository, error) {
	// Based on https://gist.github.com/PrasadG193/52faed6499d2ec739f9630b9d044ffdc
	lister := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			listOptions := &controller_runtime.ListOptions{Raw: &options, Namespace: name.Namespace, FieldSelector: fields.ParseSelectorOrDie("metadata.name=" + name.Name)}
			gitRepositories := &sourcev1.GitRepositoryList{}
			err := client.List(ctx, gitRepositories, listOptions)
			if err != nil {
				return nil, err
			}

			return gitRepositories, nil
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			listOptions := &controller_runtime.ListOptions{Raw: &options, Namespace: name.Namespace, FieldSelector: fields.ParseSelectorOrDie("metadata.name=" + name.Name)}
			gitRepositories := &sourcev1.GitRepositoryList{}
			return client.Watch(ctx, gitRepositories, listOptions)
		},
	}
	watcher, err := watchtools.NewRetryWatcherWithContext(ctx, initialVersion, lister)
	require.NoError(t, err)
	defer watcher.Stop()

	for {
		event := <-watcher.ResultChan()
		r, ok := event.Object.(*sourcev1.GitRepository)
		if !ok {
			// Not a GitRepository, likely an event.
			t.Logf("Received event: %+v", event)
			continue
		}

		t.Logf("Received GitRepository. Status: %+v", r.Status)
		for _, condition := range r.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
				return r, nil
			}
		}
	}
}

func getValuesFromRadiusGitOpsConfig(configEntry reconciler.ConfigEntry) (name string, namespace string, resourceGroup string, params string) {
	name = configEntry.Name
	namespace = configEntry.Namespace
	resourceGroup = configEntry.ResourceGroup
	params = configEntry.Params

	nameBase := strings.ReplaceAll(name, path.Ext(name), "")

	if namespace == "" {
		namespace = nameBase
	}

	if resourceGroup == "" {
		resourceGroup = nameBase
	}

	return name, namespace, resourceGroup, params
}

func ensureGitHTTPRepository(ctx context.Context, t *testing.T, opts rp.RPTestOptions, repoName string) func() {
	podName := getGitHTTPServerPodName(ctx, t, opts)
	repoRoot := getGitHTTPServerRepoRoot(ctx, t, opts, podName)

	initCmd := fmt.Sprintf(`set -euo pipefail
REPO_ROOT=%q
REPO_PATH="${REPO_ROOT}/%s.git"
rm -rf "${REPO_PATH}"
mkdir -p "${REPO_ROOT}"
git init --bare "${REPO_PATH}"
git --git-dir "${REPO_PATH}" config http.receivepack true
git --git-dir "${REPO_PATH}" config http.uploadpack true
git --git-dir "${REPO_PATH}" update-server-info
git --git-dir "${REPO_PATH}" symbolic-ref HEAD refs/heads/main || true
touch "${REPO_PATH}/git-daemon-export-ok"
chown -R nginx:nginx "${REPO_PATH}"
chmod -R 777 "${REPO_PATH}"
`,
		repoRoot,
		repoName,
	)
	_, err := execGitHTTPCommand(ctx, opts, podName, []string{"/bin/sh", "-c", initCmd})
	require.NoErrorf(t, err, "failed to initialize repository %s on git server", repoName)
	t.Logf("Initialized git repository %s on git HTTP backend pod %s (root: %s)", repoName, podName, repoRoot)

	return func() {
		cleanupPod := getGitHTTPServerPodName(ctx, t, opts)
		cleanupRoot := getGitHTTPServerRepoRoot(ctx, t, opts, cleanupPod)
		cleanupCmd := fmt.Sprintf(`set -euo pipefail
REPO_ROOT=%q
REPO_PATH="${REPO_ROOT}/%s.git"
rm -rf "${REPO_PATH}"
`, cleanupRoot, repoName)
		_, err := execGitHTTPCommand(ctx, opts, cleanupPod, []string{"/bin/sh", "-c", cleanupCmd})
		require.NoErrorf(t, err, "failed to clean up repository %s on git server", repoName)
		t.Logf("Cleaned up git repository %s from git HTTP backend pod %s (root: %s)", repoName, cleanupPod, cleanupRoot)
	}
}

func getGitHTTPServerPodName(ctx context.Context, t *testing.T, opts rp.RPTestOptions) string {
	podList, err := opts.K8sClient.CoreV1().Pods(gitServerNamespace).List(ctx, metav1.ListOptions{LabelSelector: gitServerLabelSelector})
	require.NoError(t, err)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return pod.Name
			}
		}
	}
	require.FailNowf(t, "git http backend not ready", "no ready pods found with selector %s in namespace %s", gitServerLabelSelector, gitServerNamespace)
	return ""
}

func getGitHTTPServerRepoRoot(ctx context.Context, t *testing.T, opts rp.RPTestOptions, podName string) string {
	output, err := execGitHTTPCommand(ctx, opts, podName, []string{"/bin/sh", "-c", "printenv GIT_SERVER_TEMP_DIR"})
	require.NoError(t, err)
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "/var/lib/git"
	}
	return trimmed
}

func execGitHTTPCommand(ctx context.Context, opts rp.RPTestOptions, podName string, command []string) (string, error) {
	req := opts.K8sClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(gitServerNamespace).
		SubResource("exec")

	execOptions := &corev1.PodExecOptions{
		Container: gitServerContainerName,
		Command:   command,
		Stdout:    true,
		Stderr:    true,
	}

	req.VersionedParams(execOptions, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(opts.K8sConfig, http.MethodPost, req.URL())
	if err != nil {
		return "", err
	}

	var stdout, stderr bytes.Buffer
	streamErr := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if streamErr != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return strings.TrimSpace(stdout.String()), fmt.Errorf("%w: %s", streamErr, stderrStr)
		}
		return strings.TrimSpace(stdout.String()), streamErr
	}

	return strings.TrimSpace(stdout.String()), nil
}

func waitForGitHTTPRepository(t *testing.T, repoURL, username, password string) {
	t.Helper()
	t.Logf("Waiting for git HTTP repository %s (username=%q)", repoURL, username)
	client := &http.Client{Timeout: 2 * time.Second}
	const maxLoggedBodyBytes = 256
	attempts := 0
	success := false

	require.Eventually(t, func() bool {
		attempts++
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/info/refs?service=git-receive-pack", repoURL), nil)
		if err != nil {
			t.Logf("git HTTP repo check attempt %d: failed to build request: %v", attempts, err)
			return false
		}
		if username != "" {
			req.SetBasicAuth(username, password)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Logf("git HTTP repo check attempt %d: request error: %v", attempts, err)
			return false
		}
		bodyBytes, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			t.Logf("git HTTP repo check attempt %d: failed reading response body: %v", attempts, readErr)
		}

		bodyPreview := string(bodyBytes)
		if len(bodyPreview) > maxLoggedBodyBytes {
			bodyPreview = bodyPreview[:maxLoggedBodyBytes] + fmt.Sprintf("...(%d bytes total)", len(bodyBytes))
		}
		authChallenge := resp.Header.Get("Www-Authenticate")
		t.Logf("git HTTP repo check attempt %d: status=%d, auth-challenge=%q, body-preview=%q", attempts, resp.StatusCode, authChallenge, bodyPreview)

		if resp.StatusCode == http.StatusNotFound {
			return false
		}
		if resp.StatusCode >= http.StatusBadRequest {
			return false
		}

		success = true
		return true
	}, 30*time.Second, 500*time.Millisecond, "git HTTP repository %s never became available", repoURL)

	if success {
		t.Logf("git HTTP repository %s became available after %d attempt(s)", repoURL, attempts)
	}
}
