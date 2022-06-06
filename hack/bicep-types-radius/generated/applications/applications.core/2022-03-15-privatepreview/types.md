# Applications.Core @ 2022-03-15-privatepreview

## Resource Applications.Core/applications@2022-03-15-privatepreview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [ApplicationProperties](#applicationproperties) (Required): Application properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Core/applications' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Core/containers@2022-03-15-privatepreview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [ContainerProperties](#containerproperties) (Required): Container properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Core/containers' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Core/environments@2022-03-15-privatepreview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [EnvironmentProperties](#environmentproperties) (Required): Application environment properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Core/environments' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Core/gateways@2022-03-15-privatepreview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [GatewayProperties](#gatewayproperties) (Required): Gateway properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Core/gateways' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Core/httpRoutes@2022-03-15-privatepreview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [HttpRouteProperties](#httprouteproperties) (Required): HTTP Route properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Core/httpRoutes' (ReadOnly, DeployTimeConstant): The resource type

## ApplicationProperties
### Properties
* **environment**: string (Required): The resource id of the environment linked to application.
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating': Provisioning state of the resource at the time the operation was called.

## SystemData
### Properties
* **createdAt**: string: The timestamp of resource creation (UTC).
* **createdBy**: string: The identity that created the resource.
* **createdByType**: 'Application' | 'Key' | 'ManagedIdentity' | 'User': The type of identity that created the resource.
* **lastModifiedAt**: string: The timestamp of resource last modification (UTC)
* **lastModifiedBy**: string: The identity that last modified the resource.
* **lastModifiedByType**: 'Application' | 'Key' | 'ManagedIdentity' | 'User': The type of identity that created the resource.

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## ContainerProperties
### Properties
* **application**: string (Required): Specifies resource id of the application
* **connections**: [ContainerPropertiesConnections](#containerpropertiesconnections) (Required): Dictionary of <ConnectionProperties>
* **container**: [Container](#container) (Required): Definition of a container.
* **extensions**: [Extension](#extension)[]: Extensions spec of the resource
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the resource at the time the operation was called.
* **status**: [ResourceStatus](#resourcestatus): Status of a resource.

## ContainerPropertiesConnections
### Properties
### Additional Properties
* **Additional Properties Type**: [ConnectionProperties](#connectionproperties)

## ConnectionProperties
### Properties
* **disableDefaultEnvVars**: bool
* **iam**: [IamProperties](#iamproperties)
* **source**: string (Required): The source of the connection

## IamProperties
### Properties
* **kind**: 'azure' (Required): The kind of IAM provider to configure
* **roles**: string[]: RBAC permissions to be assigned on the source resource

## Container
### Properties
* **env**: [ContainerEnv](#containerenv): Dictionary of <string>
* **image**: string (Required): The registry and image to download and run in your container
* **livenessProbe**: [HealthProbeProperties](#healthprobeproperties): Properties for readiness/liveness probe
* **ports**: [ContainerPorts](#containerports): Dictionary of <ContainerPort>
* **readinessProbe**: [HealthProbeProperties](#healthprobeproperties): Properties for readiness/liveness probe
* **volumes**: [ContainerVolumes](#containervolumes): Dictionary of <Volume>

## ContainerEnv
### Properties
### Additional Properties
* **Additional Properties Type**: string

## HealthProbeProperties
* **Discriminator**: kind

### Base Properties
* **failureThreshold**: int: Threshold number of times the probe fails after which a failure would be reported
* **initialDelaySeconds**: int: Initial delay in seconds before probing for readiness/liveness
* **periodSeconds**: int: Interval for the readiness/liveness probe in seconds
### ExecHealthProbeProperties
#### Properties
* **command**: string (Required): Command to execute to probe readiness/liveness
* **kind**: 'exec' (Required): The HealthProbeProperties kind

### HttpGetHealthProbeProperties
#### Properties
* **containerPort**: int (Required): The listening port number
* **headers**: [HttpGetHealthProbePropertiesHeaders](#httpgethealthprobepropertiesheaders): Custom HTTP headers to add to the get request
* **kind**: 'httpGet' (Required): The HealthProbeProperties kind
* **path**: string (Required): The route to make the HTTP request on

### TcpHealthProbeProperties
#### Properties
* **containerPort**: int (Required): The listening port number
* **kind**: 'tcp' (Required): The HealthProbeProperties kind


## HttpGetHealthProbePropertiesHeaders
### Properties
### Additional Properties
* **Additional Properties Type**: string

## ContainerPorts
### Properties
### Additional Properties
* **Additional Properties Type**: [ContainerPort](#containerport)

## ContainerPort
### Properties
* **containerPort**: int (Required): The listening port number
* **protocol**: 'TCP' | 'UDP' | 'grpc' | 'http': Protocol in use by the port
* **provides**: string: Specifies a route provided by this port

## ContainerVolumes
### Properties
### Additional Properties
* **Additional Properties Type**: [Volume](#volume)

## Volume
* **Discriminator**: kind

### Base Properties
* **mountPath**: string: The path where the volume is mounted
### EphemeralVolume
#### Properties
* **kind**: 'ephemeral' (Required): The Volume kind
* **managedStore**: 'disk' | 'memory' (Required): Backing store for the ephemeral volume

### PersistentVolume
#### Properties
* **kind**: 'persistent' (Required): The Volume kind
* **rbac**: 'read' | 'write': Container read/write access to the volume
* **source**: string (Required): The source of the volume


## Extension
* **Discriminator**: kind

### Base Properties
### ManualScalingExtension
#### Properties
* **kind**: 'Applications.Core/ManualScaling@v1alpha1' (Required): Specifies the extensions of a resource.
* **replicas**: int: Replica count.

### DaprSidecarExtension
#### Properties
* **appId**: string (Required): The Dapr appId. Specifies the identifier used by Dapr for service invocation.
* **appPort**: int: The Dapr appPort. Specifies the internal listening port for the application to handle requests from the Dapr sidecar.
* **config**: string: Specifies the Dapr configuration to use for the resource.
* **kind**: 'dapr.io/Sidecar@v1alpha1' (Required): Specifies the extensions of a resource.
* **protocol**: 'TCP' | 'UDP' | 'grpc' | 'http': Protocol in use by the port
* **provides**: string: Specifies the resource id of a dapr.io.InvokeHttpRoute that can route traffic to this resource.


## ResourceStatus
### Properties
* **outputResources**: any[]: Array of AnyObject

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## EnvironmentProperties
### Properties
* **compute**: [EnvironmentCompute](#environmentcompute) (Required): Compute resource used by application environment resource.
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating': Provisioning state of the resource at the time the operation was called.

## EnvironmentCompute
### Properties
* **kind**: 'kubernetes' (Required): Type of compute resource.
* **resourceId**: string (Required): The resource id of the compute resource for application environment.

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## GatewayProperties
### Properties
* **application**: string (Required): The resource id of the application linked to Gateway resource.
* **hostname**: [GatewayPropertiesHostname](#gatewaypropertieshostname): Declare hostname information for the Gateway. Leaving the hostname empty auto-assigns one: mygateway.myapp.PUBLICHOSTNAMEORIP.nip.io.
* **internal**: bool: Sets Gateway to not be exposed externally (no public IP address associated). Defaults to false (exposed to internet).
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating': Provisioning state of the resource at the time the operation was called.
* **routes**: [GatewayRoute](#gatewayroute)[]: Routes attached to this Gateway
* **status**: [ResourceStatus](#resourcestatus): Status of a resource.

## GatewayPropertiesHostname
### Properties
* **fullyQualifiedHostname**: string: Specify a fully-qualified domain name: myapp.mydomain.com. Mutually exclusive with 'prefix' and will take priority if both are defined.
* **prefix**: string: Specify a prefix for the hostname: myhostname.myapp.PUBLICHOSTNAMEORIP.nip.io. Mutually exclusive with 'fullyQualifiedHostname' and will be overridden if both are defined.

## GatewayRoute
### Properties
* **destination**: string: The HttpRoute to route to. Ex - myserviceroute.id.
* **path**: string: The path to match the incoming request path on. Ex - /myservice.
* **replacePrefix**: string: Optionally update the prefix when sending the request to the service. Ex - replacePrefix: '/' and path: '/myservice' will transform '/myservice/myroute' to '/myroute'

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## HttpRouteProperties
### Properties
* **application**: string (Required): The resource id of the application linked to HTTP Route resource.
* **hostname**: string: The internal hostname accepting traffic for the HTTP Route. Readonly.
* **port**: int: The port number for the HTTP Route. Defaults to 80. Readonly.
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating': Provisioning state of the resource at the time the operation was called.
* **scheme**: string: The scheme used for traffic. Readonly.
* **status**: [ResourceStatus](#resourcestatus): Status of a resource.
* **url**: string: A stable URL that that can be used to route traffic to a resource. Readonly.

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

