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

package k8sutil

// YAMLSeparater is the separater for fake YAML.
const YAMLSeparater = "\n---\n"

// FakeDeploymentTemplate is the template for fake deployment.
const FakeDeploymentTemplate = `
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
      serviceAccountName: %s
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
`

// FakeServiceTemplate is the template for fake service.
const FakeServiceTemplate = `
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

// FakeServiceAccountTemplate is the template for fake service account.
const FakeServiceAccountTemplate = `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: %s
  labels:
    app.kubernetes.io/name: magpie
    app.kubernetes.io/part-of: radius
`

// FakeSecretTemplate is the template for fake secret.
const FakeSecretTemplate = `
apiVersion: v1
kind: Secret
metadata:
  name: %s
type: Opaque
stringData:
  username: admin
  password: password
`

// FakeConfigMapTemplate is the template for fake config map.
const FakeConfigMapTemplate = `
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
