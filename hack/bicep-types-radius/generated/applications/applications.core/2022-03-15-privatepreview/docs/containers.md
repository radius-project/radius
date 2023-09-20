## Resource Applications.Core/containers@2022-03-15-privatepreview

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **apiVersion** | '2022-03-15-privatepreview' |  ReadOnly, DeployTimeConstant | The resource api version |
| **id** | string |  ReadOnly, DeployTimeConstant | The resource id |
| **location** | string |  Required | The geo-location where the resource lives |
| **name** | string |  Required, DeployTimeConstant | The resource name |
| **properties** | [ContainerProperties](#containerproperties) |  | Container properties |
| **systemData** | [SystemData](#systemdata) |  ReadOnly | Metadata pertaining to creation and last modification of the resource. |
| **tags** | [TrackedResourceTags](#trackedresourcetags) |  | Resource tags. |
| **type** | 'Applications.Core/containers' |  ReadOnly, DeployTimeConstant | The resource type |

## ContainerProperties

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **application** | string |  Required | Fully qualified resource ID for the application that the portable resource is consumed by |
| **connections** | [ContainerPropertiesConnections](#containerpropertiesconnections) |  | Specifies a connection to another resource. |
| **container** | [Container](#container) |  Required | Definition of a container |
| **environment** | string |  | Fully qualified resource ID for the environment that the portable resource is linked to (if applicable) |
| **extensions** | [Extension](#extension)[] |  | Extensions spec of the resource |
| **identity** | [IdentitySettings](#identitysettings) |  | IdentitySettings is the external identity setting. |
| **provisioningState** | 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' |  ReadOnly | Provisioning state of the portable resource at the time the operation was called |
| **runtimes** | [RuntimesProperties](#runtimesproperties) |  | The properties for runtime configuration |
| **status** | [ResourceStatus](#resourcestatus) |  ReadOnly | Status of a resource. |

## ContainerPropertiesConnections

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: [ConnectionProperties](#connectionproperties)

## ConnectionProperties

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **disableDefaultEnvVars** | bool |  | default environment variable override |
| **iam** | [IamProperties](#iamproperties) |  | IAM properties |
| **source** | string |  Required | The source of the connection |

## IamProperties

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **kind** | 'azure' |  Required | The kind of IAM provider to configure |
| **roles** | string[] |  | RBAC permissions to be assigned on the source resource |

## Container

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **args** | string[] |  | Arguments to the entrypoint. Overrides the container image's CMD |
| **command** | string[] |  | Entrypoint array. Overrides the container image's ENTRYPOINT |
| **env** | [ContainerEnv](#containerenv) |  | environment |
| **image** | string |  Required | The registry and image to download and run in your container |
| **imagePullPolicy** | 'Always' | 'IfNotPresent' | 'Never' |  | The image pull policy for the container |
| **livenessProbe** | [HealthProbeProperties](#healthprobeproperties) |  | Properties for readiness/liveness probe |
| **ports** | [ContainerPorts](#containerports) |  | container ports |
| **readinessProbe** | [HealthProbeProperties](#healthprobeproperties) |  | Properties for readiness/liveness probe |
| **volumes** | [ContainerVolumes](#containervolumes) |  | container volumes |
| **workingDir** | string |  | Working directory for the container |

## ContainerEnv

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: string

## HealthProbeProperties

* **Discriminator**: kind

### Base Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **failureThreshold** | int |  | Threshold number of times the probe fails after which a failure would be reported |
| **initialDelaySeconds** | int |  | Initial delay in seconds before probing for readiness/liveness |
| **periodSeconds** | int |  | Interval for the readiness/liveness probe in seconds |
| **timeoutSeconds** | int |  | Number of seconds after which the readiness/liveness probe times out. Defaults to 5 seconds |

### ExecHealthProbeProperties

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **command** | string |  Required | Command to execute to probe readiness/liveness |
| **kind** | 'exec' |  Required | Discriminator property for HealthProbeProperties. |

### HttpGetHealthProbeProperties

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **containerPort** | int |  Required | The listening port number |
| **headers** | [HttpGetHealthProbePropertiesHeaders](#httpgethealthprobepropertiesheaders) |  | Custom HTTP headers to add to the get request |
| **kind** | 'httpGet' |  Required | Discriminator property for HealthProbeProperties. |
| **path** | string |  Required | The route to make the HTTP request on |

### TcpHealthProbeProperties

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **containerPort** | int |  Required | The listening port number |
| **kind** | 'tcp' |  Required | Discriminator property for HealthProbeProperties. |


## HttpGetHealthProbePropertiesHeaders

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: string

## ContainerPorts

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: [ContainerPortProperties](#containerportproperties)

## ContainerPortProperties

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **containerPort** | int |  Required | The listening port number |
| **port** | int |  | Specifies the port that will be exposed by this container. Must be set when value different from containerPort is desired |
| **protocol** | 'TCP' | 'UDP' |  | The protocol in use by the port |
| **provides** | string |  | Specifies a route provided by this port |
| **scheme** | string |  | Specifies the URL scheme of the communication protocol. Consumers can use the scheme to construct a URL. The value defaults to 'http' or 'https' depending on the port value |

## ContainerVolumes

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: [Volume](#volume)

## Volume

* **Discriminator**: kind

### Base Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **mountPath** | string |  | The path where the volume is mounted |

### EphemeralVolume

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **kind** | 'ephemeral' |  Required | Discriminator property for Volume. |
| **managedStore** | 'disk' | 'memory' |  Required | The managed store for the ephemeral volume |

### PersistentVolume

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **kind** | 'persistent' |  Required | Discriminator property for Volume. |
| **permission** | 'read' | 'write' |  | The persistent volume permission |
| **source** | string |  Required | The source of the volume |


## Extension

* **Discriminator**: kind

### Base Properties

* **none**


### DaprSidecarExtension

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **appId** | string |  Required | The Dapr appId. Specifies the identifier used by Dapr for service invocation. |
| **appPort** | int |  | The Dapr appPort. Specifies the internal listening port for the application to handle requests from the Dapr sidecar. |
| **config** | string |  | Specifies the Dapr configuration to use for the resource. |
| **kind** | 'daprSidecar' |  Required | Discriminator property for Extension. |
| **protocol** | 'grpc' | 'http' |  | The Dapr sidecar extension protocol |

### KubernetesMetadataExtension

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **annotations** | [KubernetesMetadataExtensionAnnotations](#kubernetesmetadataextensionannotations) |  | Annotations to be applied to the Kubernetes resources output by the resource |
| **kind** | 'kubernetesMetadata' |  Required | Discriminator property for Extension. |
| **labels** | [KubernetesMetadataExtensionLabels](#kubernetesmetadataextensionlabels) |  | Labels to be applied to the Kubernetes resources output by the resource |

### KubernetesNamespaceExtension

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **kind** | 'kubernetesNamespace' |  Required | Discriminator property for Extension. |
| **namespace** | string |  Required | The namespace of the application environment. |

### ManualScalingExtension

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **kind** | 'manualScaling' |  Required | Discriminator property for Extension. |
| **replicas** | int |  Required | Replica count. |


## KubernetesMetadataExtensionAnnotations

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: string

## KubernetesMetadataExtensionLabels

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: string

## IdentitySettings

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **kind** | 'azure.com.workload' | 'undefined' |  Required | IdentitySettingKind is the kind of supported external identity setting |
| **oidcIssuer** | string |  | The URI for your compute platform's OIDC issuer |
| **resource** | string |  | The resource ID of the provisioned identity |

## RuntimesProperties

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **kubernetes** | [KubernetesRuntimeProperties](#kubernetesruntimeproperties) |  | The runtime configuration properties for Kubernetes |

## KubernetesRuntimeProperties

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **base** | string |  | The serialized YAML manifest which represents the base Kubernetes resources to deploy, such as Deployment, Service, ServiceAccount, Secrets, and ConfigMaps. |
| **pod** | [KubernetesPodSpec](#kubernetespodspec) |  | A strategic merge patch that will be applied to the PodSpec object when this container is being deployed. |

## KubernetesPodSpec

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: any

## ResourceStatus

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **compute** | [EnvironmentCompute](#environmentcompute) |  | Represents backing compute resource |
| **outputResources** | [OutputResource](#outputresource)[] |  | Properties of an output resource |

## EnvironmentCompute

* **Discriminator**: kind

### Base Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **identity** | [IdentitySettings](#identitysettings) |  | IdentitySettings is the external identity setting. |
| **resourceId** | string |  | The resource id of the compute resource for application environment. |

### KubernetesCompute

#### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **kind** | 'kubernetes' |  Required | Discriminator property for EnvironmentCompute. |
| **namespace** | string |  Required | The namespace to use for the environment. |


## OutputResource

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **id** | string |  | The UCP resource ID of the underlying resource. |
| **localId** | string |  | The logical identifier scoped to the owning Radius resource. This is only needed or used when a resource has a dependency relationship. LocalIDs do not have any particular format or meaning beyond being compared to determine dependency relationships. |
| **radiusManaged** | bool |  | Determines whether Radius manages the lifecycle of the underlying resource. |

## SystemData

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **createdAt** | string |  | The timestamp of resource creation (UTC). |
| **createdBy** | string |  | The identity that created the resource. |
| **createdByType** | 'Application' | 'Key' | 'ManagedIdentity' | 'User' |  | The type of identity that created the resource. |
| **lastModifiedAt** | string |  | The timestamp of resource last modification (UTC) |
| **lastModifiedBy** | string |  | The identity that last modified the resource. |
| **lastModifiedByType** | 'Application' | 'Key' | 'ManagedIdentity' | 'User' |  | The type of identity that created the resource. |

## TrackedResourceTags

### Properties

* **none**

### Additional Properties

* **Additional Properties Type**: string

