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
	"strings"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
)

const fakeDeployment = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: webserver
  labels:
    app: webserver
spec:
  replicas: 3
  selector:
    matchLabels:
      app: webserver
  template:
    metadata:
      labels:
        app: webserver
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
`

const fakeService = `
apiVersion: v1
kind: Service
metadata:
  name: webserver
spec:
  selector:
    app.kubernetes.io/name: webserver
  ports:
    - protocol: TCP
      port: 80
      targetPort: 9376
`

const fakeServiceWithNamespace = `
apiVersion: v1
kind: Service
metadata:
  name: webserver
  namespace: app-scoped
spec:
  selector:
    app.kubernetes.io/name: webserver
  ports:
    - protocol: TCP
      port: 80
      targetPort: 9376
`

const fakeServiceAccount = `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: webserver
  labels:
    app.kubernetes.io/name: webserver
    app.kubernetes.io/part-of: radius

`

const yamlSeparater = "\n---\n"

const fakeSecret = `
apiVersion: v1
kind: Secret
metadata:
  name: secret-admin
type: Opaque
stringData:
  username: admin
  password: password
`

const fakeConfigMap = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: webserver
  labels:
    app.kubernetes.io/name: webserver
    app.kubernetes.io/part-of: radius
data:
  appsettings.Production.json: config
`

func TestValidateAndMutateRequest_IdentityProperty(t *testing.T) {
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
						Name: "webserver",
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
						Name: "webserver",
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
			resp: rest.NewBadRequestResponse("$.properties.runtimes.base is invalid: couldn't get version/kind; json parse error: json: cannot unmarshal string into Go value of type struct { APIVersion string \"json:\\\"apiVersion,omitempty\\\"\"; Kind string \"json:\\\"kind,omitempty\\\"\" }"),
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
	validResource := &datamodel.ContainerResource{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				Name: "webserver",
			},
		},
		Properties: datamodel.ContainerProperties{},
	}

	manifestTests := []struct {
		name      string
		manifest  string
		resource  *datamodel.ContainerResource
		errString string
	}{
		{
			name:      "valid manifest with deployments/services/serviceaccounts",
			manifest:  strings.Join([]string{fakeDeployment, fakeService, fakeServiceAccount}, yamlSeparater),
			resource:  validResource,
			errString: "",
		},
		{
			name:      "valid manifest with deployments/services/secrets/configmaps",
			manifest:  strings.Join([]string{fakeDeployment, fakeService, fakeSecret, fakeSecret}, yamlSeparater),
			resource:  validResource,
			errString: "",
		},
		{
			name:      "valid manifest with multiple secrets and multiple configmaps",
			manifest:  strings.Join([]string{fakeDeployment, fakeService, fakeSecret, fakeSecret, fakeSecret, fakeConfigMap, fakeConfigMap}, yamlSeparater),
			resource:  validResource,
			errString: "",
		},
		{
			name:      "invalid manifest with multiple deployments",
			manifest:  strings.Join([]string{fakeDeployment, fakeDeployment}, yamlSeparater),
			resource:  validResource,
			errString: "only one Deployment is allowed, but the manifest includes 2 resources",
		},
		{
			name:      "invalid manifest with multiple services",
			manifest:  strings.Join([]string{fakeDeployment, fakeService, fakeService}, yamlSeparater),
			resource:  validResource,
			errString: "only one Service is allowed, but the manifest includes 2 resources",
		},
		{
			name:      "invalid manifest with multiple serviceaccounts",
			manifest:  strings.Join([]string{fakeDeployment, fakeService, fakeServiceAccount, fakeServiceAccount}, yamlSeparater),
			resource:  validResource,
			errString: "only one ServiceAccount is allowed, but the manifest includes 2 resources",
		},
		{
			name:      "invalid manifest with resource including namespace",
			manifest:  strings.Join([]string{fakeDeployment, fakeServiceWithNamespace, fakeServiceAccount}, yamlSeparater),
			resource:  validResource,
			errString: "namespace is not allowed in resources: app-scoped",
		},
	}

	for _, tc := range manifestTests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBaseManifest([]byte(tc.manifest), tc.resource)
			if tc.errString != "" {
				require.EqualError(t, err, tc.errString)
				return
			}
			require.NoError(t, err)
		})
	}
}
