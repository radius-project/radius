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
	"context"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/filesystem"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
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
	bicep          bicep.Interface
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
		FileSystem:     options.filesystem,
		Bicep:          options.bicep,
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

	// Set up mocks
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	fs := filesystem.NewMemMapFileSystem()

	archiveFetcher := NewMockArchiveFetcher(mctrl)
	archiveFetcher.EXPECT().
		Fetch("https://github.com/radius-project/example-repo.git", "sha256:1234", gomock.Any()).
		Return(nil).
		Times(1).
		Do(func(archiveURL, digest string, dir string) {
			// Write the radius-config.yaml file to the directory.
			fileContent, err := os.ReadFile(path.Join("testdata", "radius-gitops-config-basic.yaml"))
			require.NoError(t, err)
			err = fs.WriteFile(filepath.Join(dir, "radius-config.yaml"), fileContent, 0644)
			require.NoError(t, err)

			// Write the basic.bicep file to the directory.
			fileContent, err = os.ReadFile(path.Join("testdata", "radius-gitops-basic.bicep"))
			require.NoError(t, err)
			err = fs.WriteFile(filepath.Join(dir, "basic.bicep"), fileContent, 0644)
			require.NoError(t, err)
		})

	bicep := bicep.NewMockInterface(mctrl)
	bicep.EXPECT().
		Build(gomock.Any(), "--outfile", gomock.Any()).
		Return(nil, nil).
		Times(1).
		Do(func(args ...string) {
			dir := filepath.Dir(args[0])
			fileContent, err := os.ReadFile(path.Join("testdata", "radius-gitops-basic.json"))
			require.NoError(t, err)
			err = fs.WriteFile(filepath.Join(dir, "basic.json"), fileContent, 0644)
			require.NoError(t, err)
		})

	options := setupFluxControllerTestOptions{
		archiveFetcher: archiveFetcher,
		filesystem:     fs,
		bicep:          bicep,
	}

	k8sClient := SetupFluxControllerTest(t, options)

	namespaceName := "flux-basic"
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}
	err := k8sClient.Create(ctx, namespace)
	require.NoError(t, err)

	// Create a GitRepository resource on the cluster without setting Status
	gitRepoNamespacedName := types.NamespacedName{Name: "git-repo", Namespace: namespaceName}
	gitRepo := makeGitRepository(gitRepoNamespacedName, "https://github.com/radius-project/example-repo.git")
	err = k8sClient.Create(ctx, gitRepo)
	require.NoError(t, err)

	// Poll to confirm creation of GitRepository
	timeout := 5 * time.Second
	interval := 100 * time.Millisecond
	deadlineCtx, deadlineCancel := context.WithTimeout(ctx, timeout)
	defer deadlineCancel()

	err = wait.PollUntilContextTimeout(deadlineCtx, interval, timeout, true, func(_ context.Context) (bool, error) {
		err := k8sClient.Get(ctx, gitRepoNamespacedName, gitRepo)
		if err != nil {
			return false, nil // Continue polling
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

	// Now, the FluxController should reconcile the GitRepository and create the DeploymentTemplate resource.
	// Check for creation of the DeploymentTemplate resource
	deploymentTemplateName := "basic.bicep"
	deploymentTemplateNamespacedName := types.NamespacedName{Name: deploymentTemplateName, Namespace: namespaceName}

	timeout = 60 * time.Second
	interval = 1 * time.Second
	deadlineCtx, deadlineCancel = context.WithTimeout(ctx, timeout)
	defer deadlineCancel()

	deploymentTemplate := &radappiov1alpha3.DeploymentTemplate{}
	err = wait.PollUntilContextTimeout(deadlineCtx, interval, timeout, true, func(_ context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, deploymentTemplateNamespacedName, deploymentTemplate); err != nil {
			return false, nil // Continue polling if not found
		}
		return true, nil // Found DeploymentTemplate
	})
	require.NoError(t, err)
}

func makeGitRepository(namespacedName types.NamespacedName, url string) *sourcev1.GitRepository {
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
