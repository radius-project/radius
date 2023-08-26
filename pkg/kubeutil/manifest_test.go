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

package kubeutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const validManifest = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  namespace: app-scoped
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: my-service
spec:
  selector:
    app.kubernetes.io/name: MyApp
  ports:
    - protocol: TCP
      port: 80
      targetPort: 9376
`

const validManifestWithSecrets = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  namespace: app-scoped
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
---
apiVersion: v1
kind: Secret
metadata:
  name: secret-admin
type: Opaque
stringData:
  username: admin
  password: password
---
apiVersion: v1
kind: Secret
metadata:
  name: secret-user
type: Opaque
stringData:
  username: user
  password: password
`

const invalidManifest = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  namespace: app-scoped
  labels:
    app: nginx
spec:
  replicas: 3
  sele
`

func TestParseManifest(t *testing.T) {
	manifestTests := []struct {
		name      string
		manifest  string
		types     map[string]int
		errString string
	}{
		{
			name:     "valid manifest with deployments and services",
			manifest: validManifest,
			types: map[string]int{
				"apps/v1/deployment": 1,
				"core/v1/service":    1,
			},
			errString: "",
		},
		{
			name:     "valid manifest with deployments and secrets",
			manifest: validManifestWithSecrets,
			types: map[string]int{
				"apps/v1/deployment": 1,
				"core/v1/secret":     2,
			},
			errString: "",
		},
		{
			name:      "invalid manifest",
			manifest:  invalidManifest,
			errString: "error converting YAML to JSON: yaml: line 12: could not find expected ':'",
		},
	}

	for _, tc := range manifestTests {
		t.Run(tc.name, func(t *testing.T) {
			objects, err := ParseManifest([]byte(tc.manifest))
			if tc.errString != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errString)
				return
			}

			require.NoError(t, err)
			for k := range objects {
				require.Equal(t, tc.types[k], len(objects[k]))
			}
		})
	}
}
