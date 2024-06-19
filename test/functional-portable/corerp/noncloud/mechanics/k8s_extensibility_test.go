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

package mechanics_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_Kubernetes_Extensibility(t *testing.T) {
	template := "testdata/k8s-extensibility/connection-string.bicep"
	name := "corerp-mechanics-k8s-extensibility"

	expectedSecretLabels := map[string]string{
		"format": "k8s-extension",
	}

	expectedSecretAnnotations := map[string]string{
		"testAnnotation": "testAnnotation",
	}

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor:           step.NewDeployExecutor(template),
			RPResources:        &validation.RPResourceSet{},
			K8sOutputResources: loadResources("testdata/k8s-extensibility", ".output.yaml"),
			// No output resources are expected.
			SkipKubernetesOutputResourceValidation: true,
			// Added this step to verify the labels on the secret.
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				secretNamespace := "corerp-mechanics-k8s-extensibility"
				secretName := "redis-conn"
				secret, err := test.Options.K8sClient.CoreV1().Secrets(secretNamespace).Get(ctx, secretName, metav1.GetOptions{})
				require.NoError(t, err)

				labels := secret.GetLabels()
				require.Len(t, labels, 1)
				require.Equal(t, expectedSecretLabels, labels)

				annotations := secret.GetAnnotations()
				require.Len(t, annotations, 1)
				require.Equal(t, expectedSecretAnnotations, annotations)
			},
		},
	}, loadResources("testdata/k8s-extensibility", ".input.yaml")...)

	test.Test(t)
}

func loadResources(dir string, suffix string) []unstructured.Unstructured {
	objects := []unstructured.Unstructured{}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, _ error) error {
		if !strings.HasSuffix(info.Name(), suffix) {
			return nil
		}
		var r unstructured.Unstructured
		b, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		_ = yaml.Unmarshal(b, &r.Object)
		objects = append(objects, r)
		return nil
	})
	return objects
}
