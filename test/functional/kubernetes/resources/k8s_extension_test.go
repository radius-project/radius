// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/kubernetes"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestK8sExtension(t *testing.T) {
	template := "testdata/k8s-extension/connection-string.bicep"
	application := "k8s-extension"
	test := kubernetes.NewApplicationTest(t, application, []kubernetes.TestStep{
		{
			Executor:           step.NewDeployExecutor(template, functional.GetMagpieImage()),
			RadiusResources:    &validation.ResourceSet{},
			K8sOutputResources: loadResources("testdata/k8s-extension", ".output.yaml"),
		},
	}, loadResources("testdata/k8s-extension", ".input.yaml")...)

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
