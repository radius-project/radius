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
	"github.com/stretchr/testify/require"
)

const fakeDeploymentTemplate = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  %s
  labels:
    app: magpie
spec:
  replicas: 3
  selector:
    matchLabels:
      app: magpie
  template:
    metadata:
      labels:
        app: magpie
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
`

const fakeServiceTemplate = `
apiVersion: v1
kind: Service
metadata:
  name: %s
  %s
spec:
  selector:
    app.kubernetes.io/name: magpie
  ports:
    - protocol: TCP
      port: 80
      targetPort: 9376
`

const fakeServiceAccountTemplate = `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: %s
  labels:
    app.kubernetes.io/name: magpie
    app.kubernetes.io/part-of: radius
`

const yamlSeparater = "\n---\n"

const fakeSecretTemplate = `
apiVersion: v1
kind: Secret
metadata:
  name: %s
type: Opaque
stringData:
  username: admin
  password: password
`

const fakeConfigMapTemplate = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  labels:
    app.kubernetes.io/name: magpie
    app.kubernetes.io/part-of: radius
data:
  appsettings.Production.json: config
`

func TestValidateAndMutateRequest_IdentityProperty(t *testing.T) {
	fakeDeployment := fmt.Sprintf(fakeDeploymentTemplate, "magpie", "")
	fakeService := fmt.Sprintf(fakeServiceTemplate, "magpie", "")
	fakeServiceAccount := fmt.Sprintf(fakeServiceAccountTemplate, "magpie")

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
							Base: strings.Join([]string{fakeDeployment, fakeService, fakeServiceAccount}, yamlSeparater),
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
							Base: strings.Join([]string{fakeDeployment, fakeService, fakeServiceAccount}, yamlSeparater),
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
	fakeDeployment := fmt.Sprintf(fakeDeploymentTemplate, "magpie", "")
	fakeService := fmt.Sprintf(fakeServiceTemplate, "magpie", "")
	fakeServiceAccount := fmt.Sprintf(fakeServiceAccountTemplate, "magpie")
	fakeSecret := fmt.Sprintf(fakeSecretTemplate, "magpie")
	fakeConfigMap := fmt.Sprintf(fakeConfigMapTemplate, "magpie")
	fakeServiceWithNamespace := fmt.Sprintf(fakeServiceTemplate, "magpie", "namespace: app-scoped")

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
			manifest: strings.Join([]string{fakeDeployment, fakeService, fakeServiceAccount}, yamlSeparater),
			resource: validResource,
			err:      nil,
		},
		{
			name:     "valid manifest with deployments/services/secrets/configmaps",
			manifest: strings.Join([]string{fakeDeployment, fakeService, fakeSecret, fakeSecret}, yamlSeparater),
			resource: validResource,
			err:      nil,
		},
		{
			name:     "valid manifest with multiple secrets and multiple configmaps",
			manifest: strings.Join([]string{fakeDeployment, fakeService, fakeSecret, fakeSecret, fakeSecret, fakeConfigMap, fakeConfigMap}, yamlSeparater),
			resource: validResource,
			err:      nil,
		},
		{
			name:     "invalid manifest with multiple deployments",
			manifest: strings.Join([]string{fakeDeployment, fakeDeployment}, yamlSeparater),
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
			manifest: strings.Join([]string{fakeDeployment, fakeService, fakeService}, yamlSeparater),
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
			manifest: strings.Join([]string{fakeDeployment, fakeService, fakeServiceAccount, fakeServiceAccount}, yamlSeparater),
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
			manifest: strings.Join([]string{fakeDeployment, fakeServiceWithNamespace, fakeServiceAccount}, yamlSeparater),
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
			manifest: strings.Join([]string{fmt.Sprintf(fakeDeploymentTemplate, "pie", ""), fakeService, fakeServiceAccount}, yamlSeparater),
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
			manifest: strings.Join([]string{fakeDeployment, fmt.Sprintf(fakeServiceTemplate, "pie", ""), fakeServiceAccount}, yamlSeparater),
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
			manifest: strings.Join([]string{fakeDeployment, fakeService, fmt.Sprintf(fakeServiceAccountTemplate, "pie")}, yamlSeparater),
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
				},
			},
		},
		{
			name:     "invalid manifest with multiple errors",
			manifest: strings.Join([]string{fakeDeployment, fakeService, fakeService, fmt.Sprintf(fakeServiceAccountTemplate, "pie")}, yamlSeparater),
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
