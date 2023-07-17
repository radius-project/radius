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

@secure()
param kubeConfig string

@description('Specifies the prefix of radius namepsace to be scraped by prometheus.')
param prefix string = 'radius'

var podAnnotationNamespaceRegex = 'podannotationnamespaceregex = "${prefix}.*"'

import 'kubernetes@1.0.0' with {
  namespace: 'default'
  kubeConfig: kubeConfig
}

resource coreConfigMap_amaMetricsSettingsConfigmap 'core/ConfigMap@v1' = {
  metadata: {
    name: 'ama-metrics-settings-configmap'
    namespace: 'kube-system'
  }
  data: {
    'config-version': 'ver1'
    'debug-mode': 'enabled = false'
    'default-scrape-settings-enabled': 'kubelet = true\ncoredns = false\ncadvisor = true\nkubeproxy = false\napiserver = false\nkubestate = true\nnodeexporter = true\nwindowsexporter = false\nwindowskubeproxy = false\nkappiebasic = true\nprometheuscollectorhealth = false'
    'default-targets-metrics-keep-list': 'kubelet = ""\ncoredns = ""\ncadvisor = ""\nkubeproxy = ""\napiserver = ""\nkubestate = ""\nnodeexporter = ""\nwindowsexporter = ""\nwindowskubeproxy = ""\npodannotations = ""\nkappiebasic = ""\nminimalingestionprofile = true'
    'default-targets-scrape-interval-settings': 'kubelet = "30s"\ncoredns = "30s"\ncadvisor = "30s"\nkubeproxy = "30s"\napiserver = "30s"\nkubestate = "30s"\nnodeexporter = "30s"\nwindowsexporter = "30s"\nwindowskubeproxy = "30s"\nkappiebasic = "30s"\nprometheuscollectorhealth = "30s"\npodannotations = "30s"'
    'pod-annotation-based-scraping': podAnnotationNamespaceRegex
    'prometheus-collector-settings': 'cluster_alias = ""'
    'schema-version': 'v1'
  }
}
