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
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/testcontext"

	gitea "code.gitea.io/sdk/gitea"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	git "github.com/go-git/go-git/v5"
	gitobject "github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	fluxSystemNamespace             = "flux-system"
	testGitServerURLEnvVariableName = "TEST_GIT_SERVER_URL"
	testGitUsernameEnvVariableName  = "TEST_GIT_USERNAME"
	testGitEmailEnvVariableName     = "TEST_GIT_EMAIL"
)

func Test_Flux_Basic(t *testing.T) {
	testName := "flux-basic"
	steps := []GitOpsTestStep{
		{
			path: "testdata/gitops/basic",
			expectedResources: [][]string{
				{"Applications.Core/environments", "flux-basic-env"},
			},
		},
	}

	testFluxIntegration(t, testName, steps)
}

// testFluxIntegration is a helper function that runs a test for the integration of Radius and Flux.
func testFluxIntegration(t *testing.T, testName string, steps []GitOpsTestStep) {
	ctx := testcontext.New(t)
	opts := rp.NewRPTestOptions(t)

	for stepIndex, step := range steps {
		stepNumber := stepIndex + 1
		gitRepoName := fmt.Sprintf("%s-repo", testName)
		gitServerURL := os.Getenv(testGitServerURLEnvVariableName)
		gitUsername := os.Getenv(testGitUsernameEnvVariableName)
		gitEmail := os.Getenv(testGitEmailEnvVariableName)

		// Create the Gitea client.
		client, err := gitea.NewClient(gitServerURL)
		require.NoError(t, err)

		// If this is the first step, create the Git repository.
		if stepNumber == 1 {
			// Create a new repository in the Git server.
			_, _, err := client.CreateRepo(gitea.CreateRepoOption{
				Name: gitRepoName,
			})
			require.NoError(t, err)
		}

		giteaRepo, _, err := client.GetRepo(gitUsername, gitRepoName)
		require.NoError(t, err)
		gitRepositoryCloneURL := giteaRepo.CloneURL

		// Create a temporary directory to clone the repository.
		dir, err := os.MkdirTemp("", gitRepoName)
		require.NoError(t, err)
		defer os.RemoveAll(dir)

		// Clone the repository.
		clonedRepo, err := git.PlainClone(dir, false, &git.CloneOptions{
			URL: gitRepositoryCloneURL,
		})
		require.NoError(t, err)

		// Get the worktree for staging and committing changes.
		worktree, err := clonedRepo.Worktree()
		require.NoError(t, err)

		// Add all files from step.path to the repository.
		err = addFilesToRepository(worktree, step.path)
		require.NoError(t, err)

		// Commit the change.
		commit, err := worktree.Commit("Update testfile.txt", &git.CommitOptions{
			Author: &gitobject.Signature{
				Name:  gitUsername,
				Email: gitEmail,
				When:  time.Now(),
			},
		})
		require.NoError(t, err)
		t.Log(t, "Commit created:", commit)

		// Push the commit back to the repository.
		err = clonedRepo.Push(&git.PushOptions{})
		require.NoError(t, err)
		require.True(t, err != git.NoErrAlreadyUpToDate)
		t.Log(t, "Pushed changes successfully")

		fluxGitRepository := &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gitRepoName,
				Namespace: fluxSystemNamespace,
			},
			Spec: sourcev1.GitRepositorySpec{
				URL:      gitRepositoryCloneURL,
				Interval: metav1.Duration{Duration: 1 * time.Minute},
			},
		}

		err = opts.Client.Create(ctx, fluxGitRepository)
		require.NoError(t, err)

		// Wait for the GitRepository to be "Ready".

		// Wait for the DeploymentTemplate to be "Updating".

		// Wait for the DeploymentTemplate to be "Ready".
	}
}

// cleanup is a helper function that cleans up resources created during the test.
// It tries to delete all GitRepository resources, and asserts that they are deleted.
// If any resources are not deleted (including Radius resources and DeploymentTemplate resources),
// the test will fail.
func cleanup(t *testing.T) {
	// Delete the GitRepository.

	// Wait for the GitRepository to not exist.

	// Wait for the DeploymentTemplate to be "Deleting".

	// Wait for the DeploymentTemplate to not exist.
}
