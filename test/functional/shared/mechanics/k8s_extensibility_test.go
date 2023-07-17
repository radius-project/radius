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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_Kubernetes_Extensibility(t *testing.T) {
	template := "testdata/k8s-extensibility/connection-string.bicep"
	name := "corerp-mechanics-k8s-extensibility"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor:           step.NewDeployExecutor(template),
			RPResources:        &validation.RPResourceSet{},
			K8sOutputResources: loadResources("testdata/k8s-extensibility", ".output.yaml"),
			// No output resources are expected
			SkipKubernetesOutputResourceValidation: true,
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
