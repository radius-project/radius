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

// Shared Applications.Core environment that the CI functional-test workflows deploy and select.
// It is deployed declaratively, rather than with `rad env create`, so that it stays pinned to
// Applications.Core regardless of which provider the `rad env create` default targets (#11803).
// The body mirrors what `rad env create <name> --namespace <ns>` sends: location `global` and a
// Kubernetes compute with `namespace` only, with no `resourceId`. The optional cloud scopes replace
// `rad env update --azure-*`/`--aws-*`.

extension radius

@description('Name of the environment.')
param name string

@description('Kubernetes namespace the environment deploys into.')
param namespace string

@description('Azure provider scope, in the form /subscriptions/<id>/resourceGroups/<name>. Leave empty to omit the Azure provider.')
param azureScope string = ''

@description('AWS provider scope, in the form /planes/aws/aws/accounts/<id>/regions/<region>. Leave empty to omit the AWS provider.')
param awsScope string = ''

var azureProvider = empty(azureScope) ? {} : {
  azure: {
    scope: azureScope
  }
}

var awsProvider = empty(awsScope) ? {} : {
  aws: {
    scope: awsScope
  }
}

resource environment 'Applications.Core/environments@2023-10-01-preview' = {
  name: name
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      namespace: namespace
    }
    providers: union(azureProvider, awsProvider)
  }
}
