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
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
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
	"gopkg.in/yaml.v3"
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

type Step struct {
	Path string
}

func RunFluxControllerTest(t *testing.T, steps []Step) {
	ctx := testcontext.New(t)

	// Set up mocks
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	fs := filesystem.NewMemMapFileSystem()
	archiveFetcher := NewMockArchiveFetcher(mctrl)
	bicep := bicep.NewMockInterface(mctrl)

	options := setupFluxControllerTestOptions{
		archiveFetcher: archiveFetcher,
		filesystem:     fs,
		bicep:          bicep,
	}

	k8sClient := SetupFluxControllerTest(t, options)

	testGitRepoName := "example-repo"
	testGitRepoURL := "https://github.com/radius-project/example-repo.git"
	testGitRepoSHA := "sha256:1234"

	for _, step := range steps {
		archiveFetcher.EXPECT().
			Fetch(testGitRepoURL, testGitRepoSHA, gomock.Any()).
			Return(nil).
			// The archiveFetcher is called twice (TODOWILLSMITH: why?)
			AnyTimes().
			Do(func(archiveURL, digest, dir string) {
				// Copy the contents of the test data directory to the test filesystem.
				err := filepath.WalkDir(step.Path, func(srcPath string, info os.DirEntry, err error) error {
					if err != nil {
						return err
					}

					relPath, err := filepath.Rel(step.Path, srcPath)
					if err != nil {
						return err
					}

					dstPath := filepath.Join(dir, relPath)

					if info.IsDir() {
						return fs.MkdirAll(dstPath, 0755)
					}

					// Read contents of srcFile
					data, err := os.ReadFile(srcPath)
					if err != nil {
						return err
					}

					// Write contents to dstFile
					return fs.WriteFile(dstPath, data, 0644)
				})
				require.NoError(t, err)
			})

		radiusConfig := RadiusGitOpsConfig{}
		b, err := os.ReadFile(path.Join(step.Path, "radius-gitops-config.yaml"))
		require.NoError(t, err)
		err = yaml.Unmarshal(b, &radiusConfig)
		require.NoError(t, err)
		require.NotNil(t, radiusConfig)

		for _, configEntry := range radiusConfig.Config {
			name := configEntry.Name
			nameBase := strings.TrimSuffix(name, path.Ext(name))
			namespaceName := configEntry.Namespace
			if namespaceName == "" {
				namespaceName = nameBase
			}

			bicep.EXPECT().
				Build(gomock.Any(), "--outfile", gomock.Any()).
				Return(nil, nil).
				Times(1).
				Do(func(args ...string) {
					dir := filepath.Dir(args[0])
					fileContent, err := os.ReadFile(path.Join(step.Path, name))
					require.NoError(t, err)
					err = fs.WriteFile(filepath.Join(dir, fmt.Sprintf("%s.json", nameBase)), fileContent, 0644)
					require.NoError(t, err)
				})

			// Create a namespace if it does not exist
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
				},
			}
			err := k8sClient.Create(ctx, namespace)
			require.NoError(t, err)

			// Create a GitRepository resource on the cluster
			gitRepoNamespacedName := types.NamespacedName{Name: testGitRepoName, Namespace: namespaceName}
			gitRepo := makeGitRepository(gitRepoNamespacedName, testGitRepoURL)
			err = k8sClient.Create(ctx, gitRepo)
			require.NoError(t, err)

			// Wait for the GitRepository to be created
			err = waitForObjectToExist(ctx, k8sClient, gitRepoNamespacedName, gitRepo)
			require.NoError(t, err, "GitRepository was not created successfully")

			// Fetch the latest GitRepository object
			err = k8sClient.Get(ctx, gitRepoNamespacedName, gitRepo)
			require.NoError(t, err)

			// Update the Status subresource
			updatedStatus := sourcev1.GitRepositoryStatus{
				ObservedGeneration: 1,
				Artifact: &sourcev1.Artifact{
					URL:    testGitRepoURL,
					Digest: testGitRepoSHA,
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
			deploymentTemplateName := name
			deploymentTemplateNamespacedName := types.NamespacedName{Name: deploymentTemplateName, Namespace: namespaceName}
			deploymentTemplate := &radappiov1alpha3.DeploymentTemplate{}
			err = waitForObjectToExist(ctx, k8sClient, deploymentTemplateNamespacedName, deploymentTemplate)
			require.NoError(t, err)

			// Parse the DeploymentTemplate and check for the expected resources
			// TODO (willsmith)
		}
	}
}

func Test_FluxController_Basic(t *testing.T) {
	steps := []Step{
		{
			Path: "testdata/flux-basic",
		},
	}

	RunFluxControllerTest(t, steps)
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

// waitForObjectToExist waits for the specified Kubernetes object to exist
// on the cluster. It polls the cluster at intervals until the object is found
// or the timeout is reached.
func waitForObjectToExist(ctx context.Context, k8sClient k8sclient.Client, key k8sclient.ObjectKey, obj k8sclient.Object) error {
	timeout := 10 * time.Second
	interval := 1 * time.Second
	deadlineCtx, deadlineCancel := context.WithTimeout(ctx, timeout)
	defer deadlineCancel()

	return wait.PollUntilContextTimeout(deadlineCtx, interval, timeout, true, func(_ context.Context) (bool, error) {
		err := k8sClient.Get(ctx, key, obj)
		if err != nil {
			return false, nil // Continue polling
		}
		return true, nil // Found
	})
}
