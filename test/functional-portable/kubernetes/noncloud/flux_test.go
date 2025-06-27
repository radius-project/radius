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

package kubernetes_test

import (
	"context"
	"fmt"
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

	gitea "code.gitea.io/sdk/gitea"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	gitobject "github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
)

const (
	fluxSystemNamespace                 = "flux-system"
	testGitServerURLEnvVariableName     = "GITEA_SERVER_URL"
	testGitUsernameEnvVariableName      = "GITEA_USERNAME"
	testGitEmailEnvVariableName         = "GITEA_EMAIL"
	testGiteaAccessTokenEnvVariableName = "GITEA_ACCESS_TOKEN"
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

	testFluxIntegration(t, testName, steps)
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

	testFluxIntegration(t, testName, steps)
}

// testFluxIntegration is a helper function that runs a test for the integration of Radius and Flux.
func testFluxIntegration(t *testing.T, testName string, steps []GitOpsTestStep) {
	ctx := testcontext.New(t)
	opts := rp.NewRPTestOptions(t)

	// Track all namespaces created during the test for cleanup
	namespacesToCleanup := make(map[string]bool)

	gitRepoName := fmt.Sprintf("%s-repo", testName)
	gitServerURL := os.Getenv(testGitServerURLEnvVariableName)
	gitUsername := os.Getenv(testGitUsernameEnvVariableName)
	gitEmail := os.Getenv(testGitEmailEnvVariableName)
	giteaToken := os.Getenv(testGiteaAccessTokenEnvVariableName)

	// Create the Gitea client.
	client, err := gitea.NewClient(gitServerURL, gitea.SetToken(giteaToken))
	require.NoError(t, err)

	// Create a new Git repository, and delete it after the test.
	_, _, err = client.CreateRepo(gitea.CreateRepoOption{
		Name: gitRepoName,
	})
	defer func() {
		_, err := client.DeleteRepo(gitUsername, gitRepoName)
		require.NoError(t, err)
	}()
	require.NoError(t, err)

	// Get the repository to ensure it exists.
	_, _, err = client.GetRepo(gitUsername, gitRepoName)
	require.NoError(t, err)

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
	repoURL := fmt.Sprintf("%s/%s/%s.git", gitServerURL, gitUsername, gitRepoName)
	_, err = r.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{repoURL},
	})
	require.NoError(t, err)

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
			Password: giteaToken,
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
		Data: map[string][]byte{
			"bearerToken": []byte(giteaToken),
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
			URL: fmt.Sprintf("http://gitea-http.gitea.svc.cluster.local:3000/%s/%s.git", gitUsername, gitRepoName),
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
				Password: giteaToken,
			},
		})
		require.NoError(t, err)
		t.Log(t, "Pushed changes successfully")

		// Reconcile the GitRepository by updating the reconcile.fluxcd.io/requestedAt annotation.
		err = opts.Client.Get(ctx, types.NamespacedName{Name: gitRepoName, Namespace: fluxSystemNamespace}, fluxGitRepository)
		require.NoError(t, err)
		annotations := fluxGitRepository.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations["reconcile.fluxcd.io/requestedAt"] = strconv.FormatInt(time.Now().Unix(), 10)
		fluxGitRepository.SetAnnotations(annotations)
		err = opts.Client.Update(ctx, fluxGitRepository)
		require.NoError(t, err)

		radiusConfig, err := reconciler.ParseRadiusGitOpsConfig(path.Join(step.path, "radius-gitops-config.yaml"))
		require.NoError(t, err)

		for _, configEntry := range radiusConfig.Config {
			name, namespace, _, _ := getValuesFromRadiusGitOpsConfig(configEntry)
			namespacesToCleanup[namespace] = true

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

	// Find additional application namespaces that may have been created automatically
	// These follow the pattern {namespace}-{app-name}
	namespaceList, err := opts.K8sClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Logf("Failed to list namespaces for cleanup: %v", err)
	} else {
		for _, ns := range namespaceList.Items {
			nsName := ns.Name
			// Check if this namespace matches the pattern for any tracked namespace
			for trackedNs := range namespacesToCleanup {
				if strings.HasPrefix(nsName, trackedNs+"-") && nsName != trackedNs {
					t.Logf("Found additional application namespace to cleanup: %s", nsName)
					namespacesToCleanup[nsName] = true
				}
			}
		}
	}

	// Clean up namespaces at the end of the test
	for ns := range namespacesToCleanup {
		t.Logf("Cleaning up namespace: %s", ns)
		err := opts.K8sClient.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Failed to delete namespace %s: %v", ns, err)
		} else {
			// Wait for namespace to be fully deleted to avoid race conditions
			t.Logf("Waiting for namespace %s to be deleted...", ns)
			for retries := 0; retries < 60; retries++ {
				_, err := opts.K8sClient.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
				if apierrors.IsNotFound(err) {
					t.Logf("Namespace %s successfully deleted", ns)
					break
				}
				time.Sleep(time.Second)
			}
		}
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
