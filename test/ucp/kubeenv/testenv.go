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

package kubeenv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	ucpv1alpha1 "github.com/project-radius/radius/pkg/ucp/store/apiserverstore/api/ucp.dev/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// StartEnvironment creates k8s client and test environment. It returns an error if it
// fails to initialize the environment or create the client.
func StartEnvironment(crdPaths []string) (runtimeclient.Client, *envtest.Environment, error) {
	assetDir, err := getKubeAssetsDir()
	if err != nil {
		return nil, nil, err
	}

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     crdPaths,
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: assetDir,
	}

	scheme := runtime.NewScheme()

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ucpv1alpha1.AddToScheme(scheme))

	cfg, err := testEnv.Start()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize environment: %w", err)
	}

	client, err := runtimeclient.New(cfg, runtimeclient.Options{
		Scheme: scheme,
	})
	if err != nil {
		_ = testEnv.Stop()
		return nil, nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return client, testEnv, nil
}

func getKubeAssetsDir() (string, error) {
	assetsDirectory := os.Getenv("KUBEBUILDER_ASSETS")
	if assetsDirectory != "" {
		return assetsDirectory, nil
	}

	// We require one or more versions of the test assets to be installed already. This
	// will use whatever's latest of the installed versions.
	cmd := exec.Command("setup-envtest", "use", "-i", "-p", "path", "--arch", "amd64")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to call setup-envtest to find path: %w", err)
	} else {
		return out.String(), err
	}
}

// //EnsureNamespace creates a namespace if it doesn't already exist. It returns an error if the namespace cannot be
// created.
func EnsureNamespace(ctx context.Context, client runtimeclient.Client, namespace string) error {
	nsObject := v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	return client.Create(ctx, &nsObject, &runtimeclient.CreateOptions{})
}
