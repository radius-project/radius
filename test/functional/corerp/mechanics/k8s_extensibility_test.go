// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mechanics_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ktest "github.com/project-radius/radius/test/functional/kubernetes"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_Kubernetes_Extensibility(t *testing.T) {
	template := "testdata/k8s-extensibility/connection-string.bicep"
	application := "corerp-mechanics-k8s-extensibility"
	test := ktest.NewApplicationTest(t, application, []ktest.TestStep{
		{
			Executor:           step.NewDeployExecutor(template),
			RadiusResources:    &validation.ResourceSet{},
			K8sOutputResources: loadResources("testdata/k8s-extensibility", "secret.output.yaml"),

			// TODO: https://github.com/Azure/bicep/issues/7553
			// this bug blocks the use of 'existing' for Kubernetes resources. Once that's fixed we can
			// restore those resources to the test.

			// K8sOutputResources: loadResources("testdata/k8s-extensibility", ".output.yaml"),
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
		b, _ := ioutil.ReadFile(path)
		_ = yaml.Unmarshal(b, &r.Object)
		objects = append(objects, r)
		return nil
	})
	return objects
}
