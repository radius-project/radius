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

@description('Specifies the AKS cluster resource name.')
param clusterName string

@description('Specifies the AKS cluster resource location.')
param clusterLocation string

@description('Specifies comma-separated list of Kubernetes annotation keys that will be used in the resource\'s labels metric (Example: \'namespaces=[kubernetes.io/team,...],pods=[kubernetes.io/team],...\') By default the metric contains only resource name and namespace labels.')
param metricLabelsAllowlist string

@description('Specifies comma-separated list of Kubernetes annotation keys that will be used in the resource\'s labels metric (Example: \'namespaces=[kubernetes.io/team,...],pods=[kubernetes.io/team],...\') By default the metric contains only resource name and namespace labels.')
param metricAnnotationsAllowList string

// This enables the Azure Monitor for Containers addon on the AKS cluster by patching the existing cluster
// after deploying datacollection endpoint/rules and datacollection assocation on cluster resource.
resource enableMonitorAddon 'Microsoft.ContainerService/managedClusters@2023-05-01' = {
  name: clusterName
  location: clusterLocation
  properties: {
    azureMonitorProfile: {
      metrics: {
        enabled: true
        kubeStateMetrics: {
          metricLabelsAllowlist: metricLabelsAllowlist
          metricAnnotationsAllowList: metricAnnotationsAllowList
        }
      }
    }
  }
}
