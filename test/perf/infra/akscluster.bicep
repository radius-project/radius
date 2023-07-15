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

// https://github.com/Azure-Samples/aks-istio-addon-bicep/blob/main/bicep/aksCluster.bicep

// Parameters
@description('Specifies the name of the AKS cluster.')
param name string = 'aks-${uniqueString(resourceGroup().id)}'

@description('Specifies the DNS prefix specified when creating the managed cluster.')
param dnsPrefix string = name

@description('Specifies the network plugin used for building Kubernetes network. - azure or kubenet.')
@allowed([
  'azure'
  'kubenet'
])
param networkPlugin string = 'azure'

@description('Specifies the Network plugin mode used for building the Kubernetes network.')
@allowed([
  ''
  'Overlay'
])

param networkPluginMode string = ''

@description('Specifies the network policy used for building Kubernetes network. - calico or azure')
@allowed([
  'azure'
  'calico'
])
param networkPolicy string = 'azure'

@description('Specifies the CIDR notation IP range from which to assign pod IPs when kubenet is used.')
param podCidr string = '192.168.0.0/16'

@description('A CIDR notation IP range from which to assign service cluster IPs. It must not overlap with any Subnet IP ranges.')
param serviceCidr string = '172.16.0.0/16'

@description('Specifies the IP address assigned to the Kubernetes DNS service. It must be within the Kubernetes service address range specified in serviceCidr.')
param dnsServiceIP string = '172.16.0.10'

@description('Specifies the sku of the load balancer used by the virtual machine scale sets used by nodepools.')
@allowed([
  'basic'
  'standard'
])
param loadBalancerSku string = 'standard'

@description('Specifies outbound (egress) routing method. - loadBalancer or userDefinedRouting.')
@allowed([
  'loadBalancer'
  'managedNATGateway'
  'userAssignedNATGateway'
  'userDefinedRouting'
])
param outboundType string = 'loadBalancer'

@description('Specifies the tier of a managed cluster SKU: Paid or Free')
@allowed([
  'Standard'
  'Free'
])
param skuTier string = 'Standard'

@description('Specifies the version of Kubernetes specified when creating the managed cluster.')
param kubernetesVersion string

@description('Specifies the upgrade channel for auto upgrade. Allowed values include rapid, stable, patch, node-image, none.')
@allowed([
  'rapid'
  'stable'
  'patch'
  'node-image'
  'none'
])
param upgradeChannel string = 'patch'

@description('Specifies the unique name of of the system node pool profile in the context of the subscription and resource group.')
param systemAgentPoolName string = 'agentpool'

@description('Specifies the vm size of nodes in the system node pool.')
param systemAgentPoolVmSize string = 'Standard_D4as_v5'

@description('Specifies the OS Disk Size in GB to be used to specify the disk size for every machine in the system agent pool. If you specify 0, it will apply the default osDisk size according to the vmSize specified.')
param systemAgentPoolOsDiskSizeGB int = 0

@description('Specifies the OS disk type to be used for machines in a given agent pool. Allowed values are \'Ephemeral\' and \'Managed\'. If unspecified, defaults to \'Ephemeral\' when the VM supports ephemeral OS and has a cache disk larger than the requested OSDiskSizeGB. Otherwise, defaults to \'Managed\'. May not be changed after creation. - Managed or Ephemeral')
@allowed([
  'Ephemeral'
  'Managed'
])
param systemAgentPoolOsDiskType string = 'Managed'

@description('Specifies the number of agents (VMs) to host docker containers in the system node pool. Allowed values must be in the range of 1 to 100 (inclusive). The default value is 1.')
param systemAgentPoolAgentCount int = 3

@description('Specifies the OS type for the vms in the system node pool. Choose from Linux and Windows. Default to Linux.')
@allowed([
  'Linux'
  'Windows'
])
param systemAgentPoolOsType string = 'Linux'

@description('Specifies the maximum number of pods that can run on a node in the system node pool. The maximum number of pods per node in an AKS cluster is 250. The default maximum number of pods per node varies between kubenet and Azure CNI networking, and the method of cluster deployment.')
param systemAgentPoolMaxPods int = 30

@description('Specifies the maximum number of nodes for auto-scaling for the system node pool.')
param systemAgentPoolMaxCount int = 5

@description('Specifies the minimum number of nodes for auto-scaling for the system node pool.')
param systemAgentPoolMinCount int = 2

@description('Specifies whether to enable auto-scaling for the system node pool.')
param systemAgentPoolEnableAutoScaling bool = true

@description('Specifies the virtual machine scale set priority in the system node pool: Spot or Regular.')
@allowed([
  'Spot'
  'Regular'
])
param systemAgentPoolScaleSetPriority string = 'Regular'

@description('Specifies the ScaleSetEvictionPolicy to be used to specify eviction policy for spot virtual machine scale set. Default to Delete. Allowed values are Delete or Deallocate.')
@allowed([
  'Delete'
  'Deallocate'
])
param systemAgentPoolScaleSetEvictionPolicy string = 'Delete'

@description('Specifies the Agent pool node labels to be persisted across all nodes in the system node pool.')
param systemAgentPoolNodeLabels object = {}

@description('Specifies the taints added to new nodes during node pool create and scale. For example, key=value:NoSchedule.')
param systemAgentPoolNodeTaints array = ['CriticalAddonsOnly=true:NoSchedule']

@description('Determines the placement of emptyDir volumes, container runtime data root, and Kubelet ephemeral storage.')
@allowed([
  'OS'
  'Temporary'
])
param systemAgentPoolKubeletDiskType string = 'OS'

@description('Specifies the type for the system node pool: VirtualMachineScaleSets or AvailabilitySet')
@allowed([
  'VirtualMachineScaleSets'
  'AvailabilitySet'
])
param systemAgentPoolType string = 'VirtualMachineScaleSets'

@description('Specifies the availability zones for the agent nodes in the system node pool. Requirese the use of VirtualMachineScaleSets as node pool type.')
param systemAgentPoolAvailabilityZones array = [
  '1'
  '2'
  '3'
]

@description('Specifies the unique name of of the user node pool profile in the context of the subscription and resource group.')
param userAgentPoolName string = 'userpool'

@description('Specifies the vm size of nodes in the user node pool.')
param userAgentPoolVmSize string = 'Standard_D4as_v5'

@description('Specifies the OS Disk Size in GB to be used to specify the disk size for every machine in the system agent pool. If you specify 0, it will apply the default osDisk size according to the vmSize specified..')
param userAgentPoolOsDiskSizeGB int = 0

@description('Specifies the OS disk type to be used for machines in a given agent pool. Allowed values are \'Ephemeral\' and \'Managed\'. If unspecified, defaults to \'Ephemeral\' when the VM supports ephemeral OS and has a cache disk larger than the requested OSDiskSizeGB. Otherwise, defaults to \'Managed\'. May not be changed after creation. - Managed or Ephemeral')
@allowed([
  'Ephemeral'
  'Managed'
])
param userAgentPoolOsDiskType string = 'Managed'

@description('Specifies the number of agents (VMs) to host docker containers in the user node pool. Allowed values must be in the range of 1 to 100 (inclusive). The default value is 1.')
param userAgentPoolAgentCount int = 3

@description('Specifies the OS type for the vms in the user node pool. Choose from Linux and Windows. Default to Linux.')
@allowed([
  'Linux'
  'Windows'
])
param userAgentPoolOsType string = 'Linux'

@description('Specifies the maximum number of pods that can run on a node in the user node pool. The maximum number of pods per node in an AKS cluster is 250. The default maximum number of pods per node varies between kubenet and Azure CNI networking, and the method of cluster deployment.')
param userAgentPoolMaxPods int = 30

@description('Specifies the maximum number of nodes for auto-scaling for the user node pool.')
param userAgentPoolMaxCount int = 5

@description('Specifies the minimum number of nodes for auto-scaling for the user node pool.')
param userAgentPoolMinCount int = 2

@description('Specifies whether to enable auto-scaling for the user node pool.')
param userAgentPoolEnableAutoScaling bool = true

@description('Specifies the virtual machine scale set priority in the user node pool: Spot or Regular.')
@allowed([
  'Spot'
  'Regular'
])
param userAgentPoolScaleSetPriority string = 'Regular'

@description('Specifies the ScaleSetEvictionPolicy to be used to specify eviction policy for spot virtual machine scale set. Default to Delete. Allowed values are Delete or Deallocate.')
@allowed([
  'Delete'
  'Deallocate'
])
param userAgentPoolScaleSetEvictionPolicy string = 'Delete'

@description('Specifies the Agent pool node labels to be persisted across all nodes in the user node pool.')
param userAgentPoolNodeLabels object = {}

@description('Specifies the taints added to new nodes during node pool create and scale. For example, key=value:NoSchedule.')
@allowed([
  'OS'
  'Temporary'
])
param userAgentPoolNodeTaints array = []

@description('Determines the placement of emptyDir volumes, container runtime data root, and Kubelet ephemeral storage.')
param userAgentPoolKubeletDiskType string = 'OS'

@description('Specifies the type for the user node pool: VirtualMachineScaleSets or AvailabilitySet')
@allowed([
  'VirtualMachineScaleSets'
  'AvailabilitySet'
])
param userAgentPoolType string = 'VirtualMachineScaleSets'

@description('Specifies the availability zones for the agent nodes in the user node pool. Requirese the use of VirtualMachineScaleSets as node pool type.')
param userAgentPoolAvailabilityZones array = [
  '1'
  '2'
  '3'
]

@description('Specifies whether the Dapr extension is enabled or not.')
param daprEnabled bool = false

@description('Enable high availability (HA) mode for the Dapr control plane')
param daprHaEnabled bool = false

@description('Specifies whether the azurepolicy add-on is enabled or not.')
param azurePolicyEnabled bool = true

@description('Specifies whether the Azure Key Vault Provider for Secrets Store CSI Driver addon is enabled or not.')
param azureKeyvaultSecretsProviderEnabled bool = true

@description('Specifies whether the kubeDashboard add-on is enabled or not.')
param kubeDashboardEnabled bool = false

@description('Specifies whether the OIDC issuer is enabled.')
param oidcIssuerProfileEnabled bool = true

@description('Specifies the scan interval of the auto-scaler of the AKS cluster.')
param autoScalerProfileScanInterval string = '10s'

@description('Specifies the scale down delay after add of the auto-scaler of the AKS cluster.')
param autoScalerProfileScaleDownDelayAfterAdd string = '10m'

@description('Specifies the scale down delay after delete of the auto-scaler of the AKS cluster.')
param autoScalerProfileScaleDownDelayAfterDelete string = '20s'

@description('Specifies scale down delay after failure of the auto-scaler of the AKS cluster.')
param autoScalerProfileScaleDownDelayAfterFailure string = '3m'

@description('Specifies the scale down unneeded time of the auto-scaler of the AKS cluster.')
param autoScalerProfileScaleDownUnneededTime string = '10m'

@description('Specifies the scale down unready time of the auto-scaler of the AKS cluster.')
param autoScalerProfileScaleDownUnreadyTime string = '20m'

@description('Specifies the utilization threshold of the auto-scaler of the AKS cluster.')
param autoScalerProfileUtilizationThreshold string = '0.5'

@description('Specifies the max graceful termination time interval in seconds for the auto-scaler of the AKS cluster.')
param autoScalerProfileMaxGracefulTerminationSec string = '600'

@description('Specifies the resource id of the Log Analytics workspace.')
param logAnalyticsWorkspaceId string

@description('Specifies the workspace data retention in days.')
param retentionInDays int = 30

@description('Specifies the location.')
param location string = resourceGroup().location

@description('Specifies the resource tags.')
param tags object

@description('Specifies whether to enable the Azure Blob CSI Driver. The default value is false.')
param blobCSIDriverEnabled bool = false

@description('Specifies whether to enable the Azure Disk CSI Driver. The default value is true.')
param diskCSIDriverEnabled bool = true

@description('Specifies whether to enable the Azure File CSI Driver. The default value is true.')
param fileCSIDriverEnabled bool = true

@description('Specifies whether to enable the Snapshot Controller. The default value is true.')
param snapshotControllerEnabled bool = true

@description('Specifies whether to enable Defender threat detection. The default value is true.')
param defenderSecurityMonitoringEnabled bool = true

@description('Specifies whether to enable ImageCleaner on AKS cluster. The default value is false.')
param imageCleanerEnabled bool = false

@description('Specifies whether ImageCleaner scanning interval in hours.')
param imageCleanerIntervalHours int = 24

@description('Specifies whether to enable Workload Identity. The default value is false.')
param workloadIdentityEnabled bool = false

// Variables
var diagnosticSettingsName = 'diagnosticSettings'
var logCategories = [
  'kube-apiserver'
  'kube-audit'
  'kube-audit-admin'
  'kube-controller-manager'
  'kube-scheduler'
  'cluster-autoscaler'
  'cloud-controller-manager'
  'guard'
  'csi-azuredisk-controller'
  'csi-azurefile-controller'
  'csi-snapshot-controller'
]
var metricCategories = [
  'AllMetrics'
]
var logs = [for category in logCategories: {
  category: category
  enabled: true
  retentionPolicy: {
    enabled: true
    days: retentionInDays
  }
}]
var metrics = [for category in metricCategories: {
  category: category
  enabled: true
  retentionPolicy: {
    enabled: true
    days: retentionInDays
  }
}]

resource aksCluster 'Microsoft.ContainerService/managedClusters@2023-05-01' = {
  name: name
  location: location
  tags: tags
  sku: {
    name: 'Base'
    tier: skuTier
  }
  identity: {
    type: 'SystemAssigned'
  }
  properties: {
    kubernetesVersion: kubernetesVersion
    dnsPrefix: dnsPrefix
    agentPoolProfiles: [
      {
        name: toLower(systemAgentPoolName)
        count: systemAgentPoolAgentCount
        vmSize: systemAgentPoolVmSize
        osDiskSizeGB: systemAgentPoolOsDiskSizeGB
        osDiskType: systemAgentPoolOsDiskType
        maxPods: systemAgentPoolMaxPods
        osType: systemAgentPoolOsType
        maxCount: systemAgentPoolMaxCount
        minCount: systemAgentPoolMinCount
        scaleSetPriority: systemAgentPoolScaleSetPriority
        scaleSetEvictionPolicy: systemAgentPoolScaleSetEvictionPolicy
        enableAutoScaling: systemAgentPoolEnableAutoScaling
        mode: 'System'
        type: systemAgentPoolType
        availabilityZones: systemAgentPoolAvailabilityZones
        nodeLabels: systemAgentPoolNodeLabels
        nodeTaints: systemAgentPoolNodeTaints
        kubeletDiskType: systemAgentPoolKubeletDiskType
      }
      {
        name: toLower(userAgentPoolName)
        count: userAgentPoolAgentCount
        vmSize: userAgentPoolVmSize
        osDiskSizeGB: userAgentPoolOsDiskSizeGB
        osDiskType: userAgentPoolOsDiskType
        maxPods: userAgentPoolMaxPods
        osType: userAgentPoolOsType
        maxCount: userAgentPoolMaxCount
        minCount: userAgentPoolMinCount
        scaleSetPriority: userAgentPoolScaleSetPriority
        scaleSetEvictionPolicy: userAgentPoolScaleSetEvictionPolicy
        enableAutoScaling: userAgentPoolEnableAutoScaling
        mode: 'User'
        type: userAgentPoolType
        availabilityZones: userAgentPoolAvailabilityZones
        nodeLabels: userAgentPoolNodeLabels
        nodeTaints: userAgentPoolNodeTaints
        kubeletDiskType: userAgentPoolKubeletDiskType
      }
    ]
    addonProfiles: {
      omsagent: {
        enabled: true
        config: {
          logAnalyticsWorkspaceResourceID: logAnalyticsWorkspaceId
        }
      }
      azurepolicy: {
        enabled: azurePolicyEnabled
      }
      kubeDashboard: {
        enabled: kubeDashboardEnabled
      }
      azureKeyvaultSecretsProvider: {
        config: {
          enableSecretRotation: 'false'
        }
        enabled: azureKeyvaultSecretsProviderEnabled
      }
    }
    oidcIssuerProfile: {
      enabled: oidcIssuerProfileEnabled
    }
    enableRBAC: true
    networkProfile: {
      networkPlugin: networkPlugin
      networkPluginMode: networkPlugin == 'azure' ? networkPluginMode : ''
      networkPolicy: networkPolicy
      podCidr: networkPlugin == 'kubenet' || networkPluginMode == 'Overlay' ? podCidr : null
      serviceCidr: serviceCidr
      dnsServiceIP: dnsServiceIP
      outboundType: outboundType
      loadBalancerSku: loadBalancerSku
      loadBalancerProfile: null
    }
    autoUpgradeProfile: {
      upgradeChannel: upgradeChannel
    }
    autoScalerProfile: {
      'scan-interval': autoScalerProfileScanInterval
      'scale-down-delay-after-add': autoScalerProfileScaleDownDelayAfterAdd
      'scale-down-delay-after-delete': autoScalerProfileScaleDownDelayAfterDelete
      'scale-down-delay-after-failure': autoScalerProfileScaleDownDelayAfterFailure
      'scale-down-unneeded-time': autoScalerProfileScaleDownUnneededTime
      'scale-down-unready-time': autoScalerProfileScaleDownUnreadyTime
      'scale-down-utilization-threshold': autoScalerProfileUtilizationThreshold
      'max-graceful-termination-sec': autoScalerProfileMaxGracefulTerminationSec
    }
    securityProfile: {
      defender: {
        logAnalyticsWorkspaceResourceId: logAnalyticsWorkspaceId
        securityMonitoring: {
          enabled: defenderSecurityMonitoringEnabled
        }
      }
      imageCleaner: {
        enabled: imageCleanerEnabled
        intervalHours: imageCleanerIntervalHours
      }
      workloadIdentity: {
        enabled: workloadIdentityEnabled
      }
    }
    storageProfile: {
      blobCSIDriver: {
        enabled: blobCSIDriverEnabled
      }
      diskCSIDriver: {
        enabled: diskCSIDriverEnabled
      }
      fileCSIDriver: {
        enabled: fileCSIDriverEnabled
      }
      snapshotController: {
        enabled: snapshotControllerEnabled
      }
    }
  }
}

// Dapr Extension
resource daprExtension 'Microsoft.KubernetesConfiguration/extensions@2022-04-02-preview' = if (daprEnabled) {
  name: 'dapr'
  scope: aksCluster
  properties: {
    extensionType: 'Microsoft.Dapr'
    autoUpgradeMinorVersion: true
    releaseTrain: 'Stable'
    configurationSettings: {
      'global.ha.enabled': '${daprHaEnabled}'
    }
    scope: {
      cluster: {
        releaseNamespace: 'dapr-system'
      }
    }
    configurationProtectedSettings: {}
  }
}

// Diagnostic Settings
resource diagnosticSettings 'Microsoft.Insights/diagnosticSettings@2021-05-01-preview' = {
  name: diagnosticSettingsName
  scope: aksCluster
  properties: {
    workspaceId: logAnalyticsWorkspaceId
    logs: logs
    metrics: metrics
  }
}

// Output
output id string = aksCluster.id
output name string = aksCluster.name
output location string = aksCluster.location
output controlPlaneFQDN string = aksCluster.properties.fqdn
output principalIdentity string = aksCluster.identity.principalId
