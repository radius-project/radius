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
	"strconv"
	"strings"
	"testing"
	"time"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/controller/reconciler"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/testcontext"
	corev1 "k8s.io/api/core/v1"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	gitobject "github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
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

	testFluxIntegration(t, testName, steps, namespaces)
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

	testFluxIntegration(t, testName, steps, namespaces)
}

// testFluxIntegration is a helper function that runs a test for the integration of Radius and Flux.
func testFluxIntegration(t *testing.T, testName string, steps []GitOpsTestStep, namespaces []string) {
	ctx := testcontext.New(t)
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
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitRepoName,
			Namespace: fluxSystemNamespace,
		},
		Type: corev1.SecretTypeBasicAuth,
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
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitRepoName,
			Namespace: fluxSystemNamespace,
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: fmt.Sprintf(gitServerInternalRepoURLFormat, gitRepoName),
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

	for stepIndex, step := range steps {
		stepNumber := stepIndex + 1

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

		// Reconcile the GitRepository by updating the reconcile.fluxcd.io/requestedAt annotation.
		var reconciledRepo *sourcev1.GitRepository
		err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			repo := &sourcev1.GitRepository{}
			if err := opts.Client.Get(ctx, types.NamespacedName{Name: gitRepoName, Namespace: fluxSystemNamespace}, repo); err != nil {
				return err
			}
			annotations := repo.GetAnnotations()
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations["reconcile.fluxcd.io/requestedAt"] = strconv.FormatInt(time.Now().Unix(), 10)
			repo.SetAnnotations(annotations)
			if err := opts.Client.Update(ctx, repo); err != nil {
				return err
			}

			reconciledRepo = repo
			return nil
		})
		require.NoError(t, err)

		// Update our reference to the latest resource version for future delete calls.
		fluxGitRepository = reconciledRepo.DeepCopy()

		radiusConfig, err := reconciler.ParseRadiusGitOpsConfig(path.Join(step.path, "radius-gitops-config.yaml"))
		require.NoError(t, err)

		for _, configEntry := range radiusConfig.Config {
			name, namespace, _, _ := getValuesFromRadiusGitOpsConfig(configEntry)

			deploymentTemplate, err := waitForDeploymentTemplateToBeReadyWithGeneration(t, ctx, types.NamespacedName{Name: name, Namespace: namespace}, stepNumber, opts.Client)
			defer func() {
				err := opts.Client.Delete(ctx, deploymentTemplate)
				if controller_runtime.IgnoreNotFound(err) != nil {
					t.Logf("Error deleting deployment template: %v", err)
				}
			}()
			require.NoError(t, err)
		}

		scope := fmt.Sprintf("/planes/radius/local/resourceGroups/%s", step.resourceGroup)

		retryInterval := 1 * time.Second
		retryTimeout := 30 * time.Second
		start := time.Now()

		for time.Since(start) < retryTimeout {
			err = assertExpectedResourcesExist(ctx, scope, step.expectedResources, opts.Connection)
			if err == nil {
				break
			}

			err = assertExpectedResourcesToNotExist(ctx, scope, step.expectedResourcesToNotExist, opts.Connection)
			if err == nil {
				break
			}

			time.Sleep(retryInterval)
		}

		if err != nil {
			t.Fatalf("Error asserting expected resources exist: %v", err)
		}
		t.Logf("Successfully asserted expected resources exist in %s", scope)
	}

	for _, namespace := range namespaces {
		t.Logf("Deleting namespace: %s", namespace)
		deleteNamespace(ctx, t, namespace, opts)
	}
}

func waitForDeploymentTemplateToBeReadyWithGeneration(t *testing.T, ctx context.Context, name types.NamespacedName, generation int, client controller_runtime.WithWatch) (*radappiov1alpha3.DeploymentTemplate, error) {
	var timeout time.Duration = 60 * time.Second
	var interval time.Duration = 1 * time.Second

	for start := time.Now(); time.Since(start) < timeout; {
		deploymentTemplate := &radappiov1alpha3.DeploymentTemplate{}
		err := client.Get(ctx, name, deploymentTemplate)
		if err == nil {
			if deploymentTemplate.Status.Phrase == radappiov1alpha3.DeploymentTemplatePhraseReady {
				if deploymentTemplate.Status.ObservedGeneration == int64(generation) {
					t.Logf("DeploymentTemplate %s is ready with generation: %d", name.Name, deploymentTemplate.Status.ObservedGeneration)
					return deploymentTemplate, nil
				} else {
					t.Logf("DeploymentTemplate %s generation: %d, looking for %d", name.Name, deploymentTemplate.Status.ObservedGeneration, generation)
				}
			} else {
				t.Logf("DeploymentTemplate %s phrase: %s, looking for %s", name.Name, deploymentTemplate.Status.Phrase, radappiov1alpha3.DeploymentTemplatePhraseReady)
			}

			return deploymentTemplate, nil
		}

		time.Sleep(interval)
	}

	return nil, fmt.Errorf("deploymentTemplate %s not found after %f seconds", name.Name, timeout.Seconds())
}

// waitForGitRepositoryReady watches the creation of the GitRepository object
// and waits for it to be in the "Ready" state.
func waitForGitRepositoryReady(t *testing.T, ctx context.Context, name types.NamespacedName, client controller_runtime.WithWatch, initialVersion string) (*sourcev1.GitRepository, error) {
	// Based on https://gist.github.com/PrasadG193/52faed6499d2ec739f9630b9d044ffdc
	lister := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			listOptions := &controller_runtime.ListOptions{Raw: &options, Namespace: name.Namespace, FieldSelector: fields.ParseSelectorOrDie("metadata.name=" + name.Name)}
			gitRepositories := &sourcev1.GitRepositoryList{}
			err := client.List(ctx, gitRepositories, listOptions)
			if err != nil {
				return nil, err
			}

			return gitRepositories, nil
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
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
