@secure()
param kubeConfig string

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
    'pod-annotation-based-scraping': 'podannotationnamespaceregex = "radius.*"'
    'prometheus-collector-settings': 'cluster_alias = ""'
    'schema-version': 'v1'
  }
}
