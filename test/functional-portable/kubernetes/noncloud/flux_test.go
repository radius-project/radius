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
				{"Applications.Core/containers", "flux-complex-container"},
			},
		},
		{
			path:          "testdata/gitops/complex/step3",
			resourceGroup: "flux-complex",
			expectedResources: [][]string{
				{"Applications.Core/environments", "flux-complex-env"},
				{"Applications.Core/applications", "flux-complex-app"},
				{"Applications.Core/containers", "flux-complex-container-2"},
			},
		},
		{
			path:          "testdata/gitops/complex/step4",
			resourceGroup: "flux-complex",
			expectedResources: [][]string{
				{"Applications.Core/environments", "flux-complex-env"},
				{"Applications.Core/applications", "flux-complex-app"},
			},
		},
	}

	testFluxIntegration(t, testName, steps)
}

// testFluxIntegration is a helper function that runs a test for the integration of Radius and Flux.
func testFluxIntegration(t *testing.T, testName string, steps []GitOpsTestStep) {
	ctx := testcontext.New(t)
	opts := rp.NewRPTestOptions(t)

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

		// Reconcile the GitRepository
		err = opts.Client.Get(ctx, types.NamespacedName{Name: gitRepoName, Namespace: fluxSystemNamespace}, fluxGitRepository)
		require.NoError(t, err)

		// Update annotations
		annotations := fluxGitRepository.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}

		annotations["reconcile.fluxcd.io/requestedAt"] = strconv.FormatInt(time.Now().Unix(), 10)
		fluxGitRepository.SetAnnotations(annotations)

		err = opts.Client.Update(ctx, fluxGitRepository)
		require.NoError(t, err)

		// Wait for the GitRepository to be ready.
		_, err = waitForGitRepositoryReady(t, ctx, types.NamespacedName{Name: gitRepoName, Namespace: fluxSystemNamespace}, opts.Client, fluxGitRepository.ResourceVersion)
		require.NoError(t, err)

		radiusConfig, err := reconciler.ParseRadiusGitOpsConfig(path.Join(step.path, "radius-gitops-config.yaml"))
		require.NoError(t, err)

		for _, configEntry := range radiusConfig.Config {
			name, namespace, _, _ := getValuesFromRadiusGitOpsConfig(configEntry)

			deploymentTemplate, err := waitForDeploymentTemplateToExist(ctx, types.NamespacedName{Name: name, Namespace: namespace}, opts.Client)
			defer func() {
				err := opts.Client.Delete(ctx, deploymentTemplate)
				if controller_runtime.IgnoreNotFound(err) != nil {
					t.Logf("Error deleting deployment template: %v", err)
				}
			}()
			require.NoError(t, err)

			_, err = waitForDeploymentTemplateUpdating(t, ctx, types.NamespacedName{Name: name, Namespace: namespace}, opts.Client, deploymentTemplate.ResourceVersion)
			require.NoError(t, err)

			// Get the DeploymentTemplate object.
			err = opts.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deploymentTemplate)
			require.NoError(t, err)

			require.False(t, deploymentTemplate.Status.Phrase == radappiov1alpha3.DeploymentTemplatePhraseReady, "DeploymentTemplate %s is already ready", name)

			_, err = waitForDeploymentTemplateReady(t, ctx, types.NamespacedName{Name: name, Namespace: namespace}, opts.Client, deploymentTemplate.ResourceVersion)
			require.NoError(t, err)
		}

		time.Sleep(5 * time.Second)
		scope := fmt.Sprintf("/planes/radius/local/resourceGroups/%s", step.resourceGroup)
		assertExpectedResourcesExist(t, ctx, scope, step.expectedResources, opts.Connection)

		if len(step.expectedResourcesToNotExist) > 0 {
			time.Sleep(5 * time.Second)
			assertExpectedResourcesToNotExist(t, ctx, scope, step.expectedResourcesToNotExist, opts.Connection)
		}
	}
}

// waitForDeploymentTemplateToExist watches the creation of the DeploymentTemplate object
// and waits for it to exist.
// It returns the DeploymentTemplate object if it exists, or an error if it doesn't.
func waitForDeploymentTemplateToExist(ctx context.Context, name types.NamespacedName, client controller_runtime.WithWatch) (*radappiov1alpha3.DeploymentTemplate, error) {
	var deploymentTemplate *radappiov1alpha3.DeploymentTemplate
	var err error

	for range 60 {
		deploymentTemplate = &radappiov1alpha3.DeploymentTemplate{}
		err = client.Get(ctx, name, deploymentTemplate)
		if err == nil {
			return deploymentTemplate, nil
		}
		time.Sleep(1 * time.Second)
	}

	return nil, fmt.Errorf("deploymentTemplate %s not found after 60 seconds", name.Name)
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
	watcher, err := watchtools.NewRetryWatcher(initialVersion, lister)
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

func getValuesFromRadiusGitOpsConfig(configEntry reconciler.BicepConfig) (name string, namespace string, resourceGroup string, params string) {
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
