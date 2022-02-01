// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/cli/armtemplate/providers"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic/fake"
)

func Test_DeploymentEvaluator_KubernetesReference(t *testing.T) {
	options := TemplateOptions{
		SubscriptionID: "test-sub",
		ResourceGroup:  "test-group",
	}
	for _, tc := range []struct {
		name           string
		template       string
		resourceDir    string
		expectedOutput string
		expectedErr    string
	}{{
		name:           "k8s ref working",
		template:       "testdata/k8s/conn/template.json",
		resourceDir:    "testdata/k8s/conn",
		expectedOutput: "testdata/k8s/conn/output.json",
	}, {
		name:        "missing resource",
		template:    "testdata/k8s/missing-resource/template.json",
		resourceDir: "testdata/k8s/missing-resource",
		expectedErr: "testdata/k8s/missing-resource/errors.json",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			content, err := ioutil.ReadFile(tc.template)
			require.NoError(t, err)

			template, err := Parse(string(content))
			require.NoError(t, err)

			resources, err := Eval(template, options)
			require.NoError(t, err)

			evaluator := &DeploymentEvaluator{
				Template:  template,
				Options:   options,
				Deployed:  map[string]map[string]interface{}{},
				Variables: map[string]interface{}{},
				Providers: loadFakeK8sProvider(tc.resourceDir),
			}
			output := map[string]interface{}{}
			errs := []string{}
			for _, resource := range resources {
				body, err := evaluator.VisitMap(resource.Body)
				if tc.expectedErr == "" {
					require.NoError(t, err)
					output[resource.Name] = body
					continue
				}
				errs = append(errs, err.Error())
			}
			if tc.expectedErr != "" {
				expected := []string{}
				expectedContent, err := ioutil.ReadFile(tc.expectedErr)
				require.NoError(t, err)
				err = json.Unmarshal(expectedContent, &expected)
				require.NoError(t, err)
				assert.DeepEqual(t, expected, errs)
				return
			}
			if tc.expectedOutput != "" {
				expected := map[string]interface{}{}
				expectedContent, err := ioutil.ReadFile(tc.expectedOutput)
				require.NoError(t, err)
				err = json.Unmarshal(expectedContent, &expected)
				require.NoError(t, err)
				assert.DeepEqual(t, expected, output)
			}
		})
	}
}

// Main purpose of deploy evaluator is to verify reference works between deployed resources
func Test_DeploymentEvaluator_ReferenceWorks(t *testing.T) {
	content, err := ioutil.ReadFile(path.Join("testdata", "frontend-backend.json"))
	require.NoError(t, err)

	template, err := Parse(string(content))
	require.NoError(t, err)
	options := TemplateOptions{
		SubscriptionID: "test-sub",
		ResourceGroup:  "test-group",
		Parameters: map[string]map[string]interface{}{
			// Setting one required parameter, and using the default value for 'backendRoute' parameter
			"frontendRoute": {
				"value": "frontend",
			},
		},
	}

	resources, err := Eval(template, options)
	require.NoError(t, err)

	deployed := map[string]map[string]interface{}{}
	evaluator := &DeploymentEvaluator{
		Template:  template,
		Options:   options,
		Deployed:  deployed,
		Variables: map[string]interface{}{},
		Outputs:   map[string]map[string]interface{}{},
	}

	for name, variable := range template.Variables {
		value, err := evaluator.VisitValue(variable)
		require.NoError(t, err)

		evaluator.Variables[name] = value
	}
	var evaluated []Resource

	for _, resource := range resources {
		body, err := evaluator.VisitMap(resource.Body)
		require.NoError(t, err)

		resource.Body = body

		deployed[resource.ID] = body
		evaluated = append(evaluated, resource)
	}

	outputs, err := evaluator.EvaluateOutputs()
	require.NoError(t, err)
	require.Equal(t, outputs["test"]["type"].(string), "string")
	require.Equal(t, outputs["test"]["value"].(string), "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/azure-resources-container-httpbinding/HttpRoute/frontendRoute")

	application, err := GetResource(path.Join("testdata", "armevaluated", "Microsoft.CustomProviders-resourceProviders-Applicationradiusv3-azure-resources-container-httpbinding.json"))
	require.NoError(t, err)
	backend, err := GetResource(path.Join("testdata", "armevaluated", "Microsoft.CustomProviders-resourceProviders-Application-Containerradiusv3-azure-resources-container-httpbinding-backend.json"))
	require.NoError(t, err)
	frontend, err := GetResource(path.Join("testdata", "armevaluated", "Microsoft.CustomProviders-resourceProviders-Application-Containerradiusv3-azure-resources-container-httpbinding-frontend.json"))
	require.NoError(t, err)

	frontendRoute, err := GetResource(path.Join("testdata", "armevaluated", "Microsoft.CustomProviders-resourceProviders-Application-HttpRouteradiusv3-azure-resources-container-httpbinding-frontend.json"))
	require.NoError(t, err)
	backendRoute, err := GetResource(path.Join("testdata", "armevaluated", "Microsoft.CustomProviders-resourceProviders-Application-HttpRouteradiusv3-azure-resources-container-httpbinding-backend.json"))
	require.NoError(t, err)

	actual := map[string]Resource{}

	for _, resource := range evaluated {
		actual[strings.ReplaceAll(resource.Type+resource.Name, "/", "-")] = resource
	}

	expected := map[string]*Resource{
		"Microsoft.CustomProviders-resourceProviders-Application-HttpRouteradiusv3-azure-resources-container-httpbinding-frontend": frontendRoute,
		"Microsoft.CustomProviders-resourceProviders-Applicationradiusv3-azure-resources-container-httpbinding":                    application,
		"Microsoft.CustomProviders-resourceProviders-Application-HttpRouteradiusv3-azure-resources-container-httpbinding-backend":  backendRoute,
		"Microsoft.CustomProviders-resourceProviders-Application-Containerradiusv3-azure-resources-container-httpbinding-backend":  backend,
		"Microsoft.CustomProviders-resourceProviders-Application-Containerradiusv3-azure-resources-container-httpbinding-frontend": frontend,
	}

	for k, actualInfo := range actual {
		expectedInfo := expected[k]
		// Unstructured comparison causes a comparison between interface{} and a string
		// so we need to convert to JSON
		expectedUns, err := json.Marshal(expectedInfo)

		require.NoError(t, err)

		actualUns, err := json.Marshal(actualInfo)
		require.NoError(t, err)

		require.JSONEq(t, string(expectedUns), string(actualUns))
	}
}

func loadFakeK8sProvider(dir string) map[string]providers.Provider {
	objects := []runtime.Object{}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, _ error) error {
		if !strings.HasSuffix(info.Name(), ".yaml") {
			return nil
		}
		var r unstructured.Unstructured
		b, _ := ioutil.ReadFile(path)
		_ = yaml.Unmarshal(b, &r.Object)
		objects = append(objects, &r)
		return nil
	})
	fakeDynamicClient := fake.NewSimpleDynamicClient(fakeScheme(), objects...)
	provider := providers.NewK8sProvider(
		logr.FromContext(context.Background()),
		fakeDynamicClient,
		fakeRestMapper())

	return map[string]providers.Provider{
		providers.KubernetesProviderImport: provider,
	}
}

func fakeScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	return scheme
}

func fakeRestMapper() meta.RESTMapper {
	restMapper := meta.NewDefaultRESTMapper(nil)
	restMapper.AddSpecific(schema.GroupVersionKind{
		Version: "v1",
		Kind:    "Secret",
	}, schema.GroupVersionResource{
		Version:  "v1",
		Resource: "secrets",
	}, schema.GroupVersionResource{
		Version:  "v1",
		Resource: "secrets",
	}, meta.RESTScopeNamespace)
	restMapper.AddSpecific(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}, schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}, schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}, meta.RESTScopeNamespace)
	restMapper.AddSpecific(schema.GroupVersionKind{
		Version: "v1",
		Kind:    "Service",
	}, schema.GroupVersionResource{
		Version:  "v1",
		Resource: "services",
	}, schema.GroupVersionResource{
		Version:  "v1",
		Resource: "services",
	}, meta.RESTScopeNamespace)
	return restMapper
}
