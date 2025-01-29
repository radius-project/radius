/*
Copyright 2024 The Radius Authors.
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

package reconciler

import (
	"testing"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type setupFluxControllerTestOptions struct {
	archiveFetcher ArchiveFetcher
	filesystem     filesystem.FileSystem
}

func SetupFluxControllerTest(t *testing.T, options setupFluxControllerTestOptions) k8sclient.Client {
	SkipWithoutEnvironment(t)

	// For debugging, you can set uncomment this to see logs from the controller. This will cause tests to fail
	// because the logging will continue after the test completes.
	//
	// Add runtimelog "sigs.k8s.io/controller-runtime/pkg/log" to imports.
	//
	// runtimelog.SetLogger(ucplog.FromContextOrDiscard(testcontext.New(t)))

	// Shut down the manager when the test exits.
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme,
		Controller: crconfig.Controller{
			SkipNameValidation: to.Ptr(true),
		},

		// Suppress metrics in tests to avoid conflicts.
		Metrics: server.Options{
			BindAddress: "0",
		},
	})
	require.NoError(t, err)

	// Set up FluxController.
	fluxController := &FluxController{
		Client:         mgr.GetClient(),
		Filesystem:     options.filesystem,
		ArchiveFetcher: options.archiveFetcher,
	}
	err = (fluxController).SetupWithManager(mgr)
	require.NoError(t, err)

	go func() {
		err := mgr.Start(ctx)
		require.NoError(t, err)
	}()

	return mgr.GetClient()
}

func Test_FluxController_Basic(t *testing.T) {
	ctx := testcontext.New(t)

	// Set up mock filesystem and ArchiveFetcher
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	fs := filesystem.NewMemMapFileSystem()

	archiveFetcher := NewMockArchiveFetcher(mctrl)
	archiveFetcher.EXPECT().
		Fetch("https://github.com/radius-project/example-repo.git", "sha256:1234", gomock.Any()).
		Return(nil).
		Times(1).
		Do(func(archiveURL, digest string, dir string) {
			err := fs.WriteFile("radius-config.yaml", []byte("hi mom"), 0644)
			require.NoError(t, err)
		})

	options := setupFluxControllerTestOptions{
		archiveFetcher: archiveFetcher,
		filesystem:     fs,
	}

	k8sClient := SetupFluxControllerTest(t, options)

	// namespace := &corev1.Namespace{
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name: "default",
	// 	},
	// }
	// err := k8sClient.Create(ctx, namespace)
	// require.NoError(t, err)

	// Create a GitRepository resource on the cluster without setting Status
	gitRepoNamespacedName := types.NamespacedName{Name: "git-repo", Namespace: "default"}
	gitRepo := makeGitRepository(gitRepoNamespacedName, "https://github.com/radius-project/example-repo.git", "sha256:1234")
	err := k8sClient.Create(ctx, gitRepo)
	require.NoError(t, err)

	// Poll to confirm creation
	err = wait.PollImmediate(100*time.Millisecond, 5*time.Second, func() (bool, error) {
		err := k8sClient.Get(ctx, gitRepoNamespacedName, gitRepo)
		if err != nil {
			return false, nil // Keep polling
		}
		return true, nil // Found
	})
	require.NoError(t, err, "GitRepository was not created successfully")

	// Fetch the latest GitRepository object
	err = k8sClient.Get(ctx, gitRepoNamespacedName, gitRepo)
	require.NoError(t, err)

	// Update the Status subresource
	updatedStatus := sourcev1.GitRepositoryStatus{
		ObservedGeneration: 1,
		Artifact: &sourcev1.Artifact{
			URL:    "https://github.com/radius-project/example-repo.git",
			Digest: "sha256:1234",
			LastUpdateTime: metav1.Time{
				Time: time.Now(),
			},
		},
		Conditions: []metav1.Condition{
			{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "Succeeded",
				Message: "Repository is ready",
				LastTransitionTime: metav1.Time{
					Time: time.Now(),
				},
			},
		},
	}
	gitRepo.Status = updatedStatus
	err = k8sClient.Status().Update(ctx, gitRepo)
	require.NoError(t, err)

	// Retrieve the updated GitRepository to verify Status is set
	updatedGitRepo := &sourcev1.GitRepository{}
	err = k8sClient.Get(ctx, gitRepoNamespacedName, updatedGitRepo)
	require.NoError(t, err)
	require.NotEmpty(t, updatedGitRepo.Status)
	require.Equal(t, "sha256:1234", updatedGitRepo.Status.Artifact.Digest)
	require.Equal(t, metav1.ConditionTrue, updatedGitRepo.Status.Conditions[0].Status)
	require.Equal(t, "Succeeded", updatedGitRepo.Status.Conditions[0].Reason)
	require.Equal(t, "Repository is ready", updatedGitRepo.Status.Conditions[0].Message)

	// Continue with your assertions or further test logic...
}

func makeGitRepository(namespacedName types.NamespacedName, url, digest string) *sourcev1.GitRepository {
	return &sourcev1.GitRepository{
		TypeMeta: metav1.TypeMeta{
			Kind:       "GitRepository",
			APIVersion: "source.toolkit.fluxcd.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: url,
		},
	}
}

// func waitForGitRepositoryStateReady(t *testing.T, client k8sclient.Client, name types.NamespacedName) *sourcev1.GitRepositoryStatus {
// 	ctx := testcontext.New(t)

// 	logger := t
// 	status := &sourcev1.GitRepositoryStatus{}
// 	require.EventuallyWithTf(t, func(t *assert.CollectT) {
// 		logger.Logf("Fetching GitRepository: %+v", name)
// 		current := &sourcev1.GitRepository{}
// 		err := client.Get(ctx, name, current)
// 		require.NoError(t, err)

// 		status = &current.Status
// 		logger.Logf("GitRepository.Status: %+v", current.Status)
// 		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

// 		assert.Equal(t, "True", current.Status.Conditions[0].Status)
// 	}, deploymentTemplateTestWaitDuration, deploymentTemplateTestWaitInterval, "failed to enter ready state")

// 	return status
// }
