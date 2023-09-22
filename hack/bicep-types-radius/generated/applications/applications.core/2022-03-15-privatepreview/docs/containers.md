### Top-Level Resource

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **apiVersion** | '2022-03-15-privatepreview' | The resource api version  (_ReadOnly, DeployTimeConstant_) |
| **id** | string | The resource id  (_ReadOnly, DeployTimeConstant_) |
| **location** | string | The geo-location where the resource lives  (_Required_) |
| **name** | string | The resource name  (_Required, DeployTimeConstant_) |
| **properties** | [ContainerProperties](#containerproperties) | Container properties |
| **systemData** | [SystemData](#systemdata) | Metadata pertaining to creation and last modification of the resource.  (_ReadOnly_) |
| **tags** | [TrackedResourceTags](#trackedresourcetags) | Resource tags. |
| **type** | 'Applications.Core/containers' | The resource type  (_ReadOnly, DeployTimeConstant_) |

### ContainerProperties

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **application** | string | Fully qualified resource ID for the application that the portable resource is consumed by  (_Required_) |
| **connections** | [ContainerPropertiesConnections](#containerpropertiesconnections) | Specifies a connection to another resource. |
| **container** | [Container](#container) | Definition of a container  (_Required_) |
| **environment** | string | Fully qualified resource ID for the environment that the portable resource is linked to (if applicable) |
| **extensions** | [Extension](#extension)[] | Extensions spec of the resource |
| **identity** | [IdentitySettings](#identitysettings) | IdentitySettings is the external identity setting. |
| **provisioningState** | 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' | Provisioning state of the portable resource at the time the operation was called  (_ReadOnly_) |
| **runtimes** | [RuntimesProperties](#runtimesproperties) | The properties for runtime configuration |
| **status** | [ResourceStatus](#resourcestatus) | Status of a resource.  (_ReadOnly_) |

### ContainerPropertiesConnections

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: [ConnectionProperties](#connectionproperties)

### ConnectionProperties

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **disableDefaultEnvVars** | bool | default environment variable override |
| **iam** | [IamProperties](#iamproperties) | IAM properties |
| **source** | string | The source of the connection  (_Required_) |

### IamProperties

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kind** | 'azure' | The kind of IAM provider to configure  (_Required_) |
| **roles** | string[] | RBAC permissions to be assigned on the source resource |

### Container

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **args** | string[] | Arguments to the entrypoint. Overrides the container image's CMD |
| **command** | string[] | Entrypoint array. Overrides the container image's ENTRYPOINT |
| **env** | [ContainerEnv](#containerenv) | environment |
| **image** | string | The registry and image to download and run in your container  (_Required_) |
| **imagePullPolicy** | 'Always' | 'IfNotPresent' | 'Never' | The image pull policy for the container |
| **livenessProbe** | [HealthProbeProperties](#healthprobeproperties) | Properties for readiness/liveness probe |
| **ports** | [ContainerPorts](#containerports) | container ports |
| **readinessProbe** | [HealthProbeProperties](#healthprobeproperties) | Properties for readiness/liveness probe |
| **volumes** | [ContainerVolumes](#containervolumes) | container volumes |
| **workingDir** | string | Working directory for the container |

### ContainerEnv

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: string

### HealthProbeProperties

* **Discriminator**: kind

#### Base Properties

| Property | Type | Description |
|----------|------|-------------|
| **failureThreshold** | int | Threshold number of times the probe fails after which a failure would be reported |
| **initialDelaySeconds** | int | Initial delay in seconds before probing for readiness/liveness |
| **periodSeconds** | int | Interval for the readiness/liveness probe in seconds |
| **timeoutSeconds** | int | Number of seconds after which the readiness/liveness probe times out. Defaults to 5 seconds |

#### ExecHealthProbeProperties

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **command** | string | Command to execute to probe readiness/liveness  (_Required_) |
| **kind** | 'exec' | Discriminator property for HealthProbeProperties.  (_Required_) |

#### HttpGetHealthProbeProperties

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **containerPort** | int | The listening port number  (_Required_) |
| **headers** | [HttpGetHealthProbePropertiesHeaders](#httpgethealthprobepropertiesheaders) | Custom HTTP headers to add to the get request |
| **kind** | 'httpGet' | Discriminator property for HealthProbeProperties.  (_Required_) |
| **path** | string | The route to make the HTTP request on  (_Required_) |

#### TcpHealthProbeProperties

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **containerPort** | int | The listening port number  (_Required_) |
| **kind** | 'tcp' | Discriminator property for HealthProbeProperties.  (_Required_) |


### HttpGetHealthProbePropertiesHeaders

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: string

### ContainerPorts

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: [ContainerPortProperties](#containerportproperties)

### ContainerPortProperties

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **containerPort** | int | The listening port number  (_Required_) |
| **port** | int | Specifies the port that will be exposed by this container. Must be set when value different from containerPort is desired |
| **protocol** | 'TCP' | 'UDP' | The protocol in use by the port |
| **provides** | string | Specifies a route provided by this port |
| **scheme** | string | Specifies the URL scheme of the communication protocol. Consumers can use the scheme to construct a URL. The value defaults to 'http' or 'https' depending on the port value |

### ContainerVolumes

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: [Volume](#volume)

### Volume

* **Discriminator**: kind

#### Base Properties

| Property | Type | Description |
|----------|------|-------------|
| **mountPath** | string | The path where the volume is mounted |

#### EphemeralVolume

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kind** | 'ephemeral' | Discriminator property for Volume.  (_Required_) |
| **managedStore** | 'disk' | 'memory' | The managed store for the ephemeral volume  (_Required_) |

#### PersistentVolume

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kind** | 'persistent' | Discriminator property for Volume.  (_Required_) |
| **permission** | 'read' | 'write' | The persistent volume permission |
| **source** | string | The source of the volume  (_Required_) |


### Extension

* **Discriminator**: kind

#### Base Properties

* **none**


#### DaprSidecarExtension

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **appId** | string | The Dapr appId. Specifies the identifier used by Dapr for service invocation.  (_Required_) |
| **appPort** | int | The Dapr appPort. Specifies the internal listening port for the application to handle requests from the Dapr sidecar. |
| **config** | string | Specifies the Dapr configuration to use for the resource. |
| **kind** | 'daprSidecar' | Discriminator property for Extension.  (_Required_) |
| **protocol** | 'grpc' | 'http' | The Dapr sidecar extension protocol |

#### KubernetesMetadataExtension

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **annotations** | [KubernetesMetadataExtensionAnnotations](#kubernetesmetadataextensionannotations) | Annotations to be applied to the Kubernetes resources output by the resource |
| **kind** | 'kubernetesMetadata' | Discriminator property for Extension.  (_Required_) |
| **labels** | [KubernetesMetadataExtensionLabels](#kubernetesmetadataextensionlabels) | Labels to be applied to the Kubernetes resources output by the resource |

#### KubernetesNamespaceExtension

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kind** | 'kubernetesNamespace' | Discriminator property for Extension.  (_Required_) |
| **namespace** | string | The namespace of the application environment.  (_Required_) |

#### ManualScalingExtension

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kind** | 'manualScaling' | Discriminator property for Extension.  (_Required_) |
| **replicas** | int | Replica count.  (_Required_) |


### KubernetesMetadataExtensionAnnotations

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: string

### KubernetesMetadataExtensionLabels

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: string

### IdentitySettings

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kind** | 'azure.com.workload' | 'undefined' | IdentitySettingKind is the kind of supported external identity setting  (_Required_) |
| **oidcIssuer** | string | The URI for your compute platform's OIDC issuer |
| **resource** | string | The resource ID of the provisioned identity |

### RuntimesProperties

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kubernetes** | [KubernetesRuntimeProperties](#kubernetesruntimeproperties) | The runtime configuration properties for Kubernetes |

### KubernetesRuntimeProperties

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **base** | string | The serialized YAML manifest which represents the base Kubernetes resources to deploy, such as Deployment, Service, ServiceAccount, Secrets, and ConfigMaps. |
| **pod** | [KubernetesPodSpec](#kubernetespodspec) | A strategic merge patch that will be applied to the PodSpec object when this container is being deployed. |

### KubernetesPodSpec

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: any

### ResourceStatus

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **compute** | [EnvironmentCompute](#environmentcompute) | Represents backing compute resource |
| **outputResources** | [OutputResource](#outputresource)[] | Properties of an output resource |

### EnvironmentCompute

* **Discriminator**: kind

#### Base Properties

| Property | Type | Description |
|----------|------|-------------|
| **identity** | [IdentitySettings](#identitysettings) | IdentitySettings is the external identity setting. |
| **resourceId** | string | The resource id of the compute resource for application environment. |

#### KubernetesCompute

##### Properties

| Property | Type | Description |
|----------|------|-------------|
| **kind** | 'kubernetes' | Discriminator property for EnvironmentCompute.  (_Required_) |
| **namespace** | string | The namespace to use for the environment.  (_Required_) |


### OutputResource

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **id** | string | The UCP resource ID of the underlying resource. |
| **localId** | string | The logical identifier scoped to the owning Radius resource. This is only needed or used when a resource has a dependency relationship. LocalIDs do not have any particular format or meaning beyond being compared to determine dependency relationships. |
| **radiusManaged** | bool | Determines whether Radius manages the lifecycle of the underlying resource. |

### SystemData

#### Properties

| Property | Type | Description |
|----------|------|-------------|
| **createdAt** | string | The timestamp of resource creation (UTC). |
| **createdBy** | string | The identity that created the resource. |
| **createdByType** | 'Application' | 'Key' | 'ManagedIdentity' | 'User' | The type of identity that created the resource. |
| **lastModifiedAt** | string | The timestamp of resource last modification (UTC) |
| **lastModifiedBy** | string | The identity that last modified the resource. |
| **lastModifiedByType** | 'Application' | 'Key' | 'ManagedIdentity' | 'User' | The type of identity that created the resource. |

### TrackedResourceTags

#### Properties

* **none**

#### Additional Properties

* **Additional Properties Type**: string

