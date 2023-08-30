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

package containers

import (
	"context"
	"fmt"
	"strings"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
)

func TestValidateAndMutateRequest_IdentityProperty(t *testing.T) {
	fakeDeployment := fmt.Sprintf(k8sutil.FakeDeploymentTemplate, "magpie", "", "magpie")
	fakeService := fmt.Sprintf(k8sutil.FakeServiceTemplate, "magpie", "")
	fakeServiceAccount := fmt.Sprintf(k8sutil.FakeServiceAccountTemplate, "magpie")

	requestTests := []struct {
		desc            string
		newResource     *datamodel.ContainerResource
		oldResource     *datamodel.ContainerResource
		mutatedResource *datamodel.ContainerResource
		resp            rest.Response
	}{
		{
			desc: "nil identity",
			newResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{},
			},
			oldResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{},
			},
			mutatedResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{},
			},
			resp: nil,
		},
		{
			desc: "user defined identity not supported",
			newResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{
					Identity: &rpv1.IdentitySettings{
						Kind:       rpv1.AzureIdentityWorkload,
						OIDCIssuer: "https://issuer",
					},
				},
			},
			resp: rest.NewBadRequestResponse("User-defined identity in Applications.Core/containers is not supported."),
		},
		{
			desc: "valid identity",
			newResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{},
			},
			oldResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{
					Identity: &rpv1.IdentitySettings{
						Kind:       rpv1.AzureIdentityWorkload,
						OIDCIssuer: "https://oidcurl/id",
						Resource:   "identity-resource-id",
					},
				},
			},
			mutatedResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{
					Identity: &rpv1.IdentitySettings{
						Kind:       rpv1.AzureIdentityWorkload,
						OIDCIssuer: "https://oidcurl/id",
						Resource:   "identity-resource-id",
					},
				},
			},
			resp: nil,
		},
		{
			desc: "valid runtime.kubernetes.base",
			newResource: &datamodel.ContainerResource{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						Name: "magpie",
					},
				},
				Properties: datamodel.ContainerProperties{
					Runtimes: &datamodel.RuntimeProperties{
						Kubernetes: &datamodel.KubernetesRuntime{
							Base: strings.Join([]string{fakeDeployment, fakeService, fakeServiceAccount}, k8sutil.YAMLSeparater),
						},
					},
				},
			},
			mutatedResource: &datamodel.ContainerResource{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						Name: "magpie",
					},
				},
				Properties: datamodel.ContainerProperties{
					Runtimes: &datamodel.RuntimeProperties{
						Kubernetes: &datamodel.KubernetesRuntime{
							Base: strings.Join([]string{fakeDeployment, fakeService, fakeServiceAccount}, k8sutil.YAMLSeparater),
						},
					},
				},
			},
			resp: nil,
		},
		{
			desc: "empty runtime.kubernetes.base",
			newResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{
					Runtimes: &datamodel.RuntimeProperties{
						Kubernetes: &datamodel.KubernetesRuntime{
							Base: "",
						},
					},
				},
			},
			mutatedResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{
					Runtimes: &datamodel.RuntimeProperties{
						Kubernetes: &datamodel.KubernetesRuntime{
							Base: "",
						},
					},
				},
			},
			resp: nil,
		},
		{
			desc: "invalid runtime.kubernetes.base",
			newResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{
					Runtimes: &datamodel.RuntimeProperties{
						Kubernetes: &datamodel.KubernetesRuntime{
							Base: "invalid",
						},
					},
				},
			},
			mutatedResource: &datamodel.ContainerResource{
				Properties: datamodel.ContainerProperties{
					Runtimes: &datamodel.RuntimeProperties{
						Kubernetes: &datamodel.KubernetesRuntime{
							Base: "invalid",
						},
					},
				},
			},
			resp: rest.NewBadRequestARMResponse(v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    v1.CodeInvalidRequestContent,
					Target:  "$.properties.runtimes.kubernetes.base",
					Message: "couldn't get version/kind; json parse error: json: cannot unmarshal string into Go value of type struct { APIVersion string \"json:\\\"apiVersion,omitempty\\\"\"; Kind string \"json:\\\"kind,omitempty\\\"\" }",
				},
			}),
		},
	}

	for _, tc := range requestTests {
		t.Run(tc.desc, func(t *testing.T) {
			r, err := ValidateAndMutateRequest(context.Background(), tc.newResource, tc.oldResource, nil)

			require.NoError(t, err)
			if tc.resp != nil {
				require.Equal(t, tc.resp, r)
			} else {
				require.Nil(t, r)
				require.Equal(t, tc.mutatedResource, tc.newResource)
			}
		})
	}
}

func TestValidateManifest(t *testing.T) {
	fakeDeployment := fmt.Sprintf(k8sutil.FakeDeploymentTemplate, "magpie", "", "magpie")
	fakeService := fmt.Sprintf(k8sutil.FakeServiceTemplate, "magpie", "")
	fakeServiceAccount := fmt.Sprintf(k8sutil.FakeServiceAccountTemplate, "magpie")
	fakeSecret := fmt.Sprintf(k8sutil.FakeSecretTemplate, "magpie")
	fakeConfigMap := fmt.Sprintf(k8sutil.FakeSecretTemplate, "magpie")
	fakeServiceWithNamespace := fmt.Sprintf(k8sutil.FakeServiceTemplate, "magpie", "namespace: app-scoped")

	validResource := &datamodel.ContainerResource{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				Name: "magpie",
			},
		},
		Properties: datamodel.ContainerProperties{},
	}

	manifestTests := []struct {
		name     string
		manifest string
		resource *datamodel.ContainerResource
		err      error
	}{
		{
			name:     "valid manifest with deployments/services/serviceaccounts",
			manifest: strings.Join([]string{fakeDeployment, fakeService, fakeServiceAccount}, k8sutil.YAMLSeparater),
			resource: validResource,
			err:      nil,
		},
		{
			name:     "valid manifest with deployments/services/secrets/configmaps",
			manifest: strings.Join([]string{fakeDeployment, fakeService, fakeSecret, fakeSecret}, k8sutil.YAMLSeparater),
			resource: validResource,
			err:      nil,
		},
		{
			name:     "valid manifest with multiple secrets and multiple configmaps",
			manifest: strings.Join([]string{fakeDeployment, fakeService, fakeSecret, fakeSecret, fakeSecret, fakeConfigMap, fakeConfigMap}, k8sutil.YAMLSeparater),
			resource: validResource,
			err:      nil,
		},
		{
			name:     "invalid manifest with multiple deployments",
			manifest: strings.Join([]string{fakeDeployment, fakeDeployment}, k8sutil.YAMLSeparater),
			resource: validResource,
			err: v1.ErrorDetails{
				Code:    v1.CodeInvalidRequestContent,
				Target:  manifestTargetProperty,
				Message: "The manifest includes invalid resources.",
				Details: []v1.ErrorDetails{
					{
						Code:    v1.CodeInvalidRequestContent,
						Target:  manifestTargetProperty,
						Message: "only one Deployment is allowed, but the manifest includes 2 resources.",
					},
				},
			},
		},
		{
			name:     "invalid manifest with multiple services",
			manifest: strings.Join([]string{fakeDeployment, fakeService, fakeService}, k8sutil.YAMLSeparater),
			resource: validResource,
			err: v1.ErrorDetails{
				Code:    v1.CodeInvalidRequestContent,
				Target:  manifestTargetProperty,
				Message: "The manifest includes invalid resources.",
				Details: []v1.ErrorDetails{
					{
						Code:    v1.CodeInvalidRequestContent,
						Target:  manifestTargetProperty,
						Message: "only one Service is allowed, but the manifest includes 2 resources.",
					},
				},
			},
		},
		{
			name:     "invalid manifest with multiple serviceaccounts",
			manifest: strings.Join([]string{fakeDeployment, fakeService, fakeServiceAccount, fakeServiceAccount}, k8sutil.YAMLSeparater),
			resource: validResource,
			err: v1.ErrorDetails{
				Code:    v1.CodeInvalidRequestContent,
				Target:  manifestTargetProperty,
				Message: "The manifest includes invalid resources.",
				Details: []v1.ErrorDetails{
					{
						Code:    v1.CodeInvalidRequestContent,
						Target:  manifestTargetProperty,
						Message: "only one ServiceAccount is allowed, but the manifest includes 2 resources.",
					},
				},
			},
		},
		{
			name:     "invalid manifest with resource including namespace",
			manifest: strings.Join([]string{fakeDeployment, fakeServiceWithNamespace, fakeServiceAccount}, k8sutil.YAMLSeparater),
			resource: validResource,
			err: v1.ErrorDetails{
				Code:    v1.CodeInvalidRequestContent,
				Target:  manifestTargetProperty,
				Message: "The manifest includes invalid resources.",
				Details: []v1.ErrorDetails{
					{
						Code:    v1.CodeInvalidRequestContent,
						Target:  manifestTargetProperty,
						Message: "namespace is not allowed in resources: app-scoped.",
					},
				},
			},
		},
		{
			name:     "invalid manifest with unmatched deployment name",
			manifest: strings.Join([]string{fmt.Sprintf(k8sutil.FakeDeploymentTemplate, "pie", "", "magpie"), fakeService, fakeServiceAccount}, k8sutil.YAMLSeparater),
			resource: validResource,
			err: v1.ErrorDetails{
				Code:    v1.CodeInvalidRequestContent,
				Target:  manifestTargetProperty,
				Message: "The manifest includes invalid resources.",
				Details: []v1.ErrorDetails{
					{
						Code:    v1.CodeInvalidRequestContent,
						Target:  manifestTargetProperty,
						Message: "Deployment name pie in manifest does not match resource name magpie.",
					},
				},
			},
		},
		{
			name:     "invalid manifest with unmatched service name",
			manifest: strings.Join([]string{fakeDeployment, fmt.Sprintf(k8sutil.FakeServiceTemplate, "pie", ""), fakeServiceAccount}, k8sutil.YAMLSeparater),
			resource: validResource,
			err: v1.ErrorDetails{
				Code:    v1.CodeInvalidRequestContent,
				Target:  manifestTargetProperty,
				Message: "The manifest includes invalid resources.",
				Details: []v1.ErrorDetails{
					{
						Code:    v1.CodeInvalidRequestContent,
						Target:  manifestTargetProperty,
						Message: "Service name pie in manifest does not match resource name magpie.",
					},
				},
			},
		},
		{
			name:     "invalid manifest with unmatched serviceaccount name",
			manifest: strings.Join([]string{fakeDeployment, fakeService, fmt.Sprintf(k8sutil.FakeServiceAccountTemplate, "pie")}, k8sutil.YAMLSeparater),
			resource: validResource,
			err: v1.ErrorDetails{
				Code:    v1.CodeInvalidRequestContent,
				Target:  manifestTargetProperty,
				Message: "The manifest includes invalid resources.",
				Details: []v1.ErrorDetails{
					{
						Code:    v1.CodeInvalidRequestContent,
						Target:  manifestTargetProperty,
						Message: "ServiceAccount name pie in manifest does not match resource name magpie.",
					},
					{
						Code:    v1.CodeInvalidRequestContent,
						Target:  manifestTargetProperty,
						Message: "ServiceAccount name magpie in PodSpec does not match the name pie in ServiceAccount.",
					},
				},
			},
		},
		{
			name:     "invalid manifest with multiple errors",
			manifest: strings.Join([]string{fakeDeployment, fakeService, fakeService, fmt.Sprintf(k8sutil.FakeServiceAccountTemplate, "pie")}, k8sutil.YAMLSeparater),
			resource: validResource,
			err: v1.ErrorDetails{
				Code:    v1.CodeInvalidRequestContent,
				Target:  manifestTargetProperty,
				Message: "The manifest includes invalid resources.",
				Details: []v1.ErrorDetails{
					{
						Code:    v1.CodeInvalidRequestContent,
						Target:  manifestTargetProperty,
						Message: "only one Service is allowed, but the manifest includes 2 resources.",
					},
					{
						Code:    v1.CodeInvalidRequestContent,
						Target:  manifestTargetProperty,
						Message: "ServiceAccount name pie in manifest does not match resource name magpie.",
					},
					{
						Code:    v1.CodeInvalidRequestContent,
						Target:  manifestTargetProperty,
						Message: "ServiceAccount name magpie in PodSpec does not match the name pie in ServiceAccount.",
					},
				},
			},
		},
	}

	for _, tc := range manifestTests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBaseManifest([]byte(tc.manifest), tc.resource)
			if tc.err != nil {
				expected := tc.err.(v1.ErrorDetails)
				actual := err.(v1.ErrorDetails)
				require.Equal(t, expected.Code, actual.Code)
				require.Equal(t, expected.Target, actual.Target)
				require.Equal(t, expected.Message, actual.Message)
				require.ElementsMatch(t, expected.Details, actual.Details)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
