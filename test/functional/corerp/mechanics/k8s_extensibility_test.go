// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mechanics_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_Kubernetes_Extensibility(t *testing.T) {
	//t.Skip("Skipping Kubernetes extensibility test until we have a way to test it in CI")
	template := "testdata/k8s-extensibility/connection-string.bicep"
	name := "corerp-mechanics-k8s-extensibility"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor:           step.NewDeployExecutor(template),
			CoreRPResources:    &validation.CoreRPResourceSet{},
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
		b, _ := os.ReadFile(path)
		_ = yaml.Unmarshal(b, &r.Object)
		objects = append(objects, r)
		return nil
	})
	return objects
}
