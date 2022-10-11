# Applications.Core @ 2022-03-15-privatepreview

## Resource Applications.Core/applications@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
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
* **Valid Scope(s)**: Unknown
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
* **Valid Scope(s)**: Unknown
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
* **Valid Scope(s)**: Unknown
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
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [HttpRouteProperties](#httprouteproperties) (Required): HTTP Route properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Core/httpRoutes' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Core/volumes@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [VolumeProperties](#volumeproperties) (Required)
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Core/volumes' (ReadOnly, DeployTimeConstant): The resource type

## ApplicationProperties
### Properties
* **environment**: string (Required): The resource id of the environment linked to application.
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the resource at the time the operation was called.

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
* **connections**: [ContainerPropertiesConnections](#containerpropertiesconnections): Dictionary of <ConnectionProperties>
* **container**: [Container](#container) (Required): Definition of a container.
* **extensions**: [Extension](#extension)[]: Extensions spec of the resource
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the resource at the time the operation was called.
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.

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
### DaprSidecarExtension
#### Properties
* **appId**: string (Required): The Dapr appId. Specifies the identifier used by Dapr for service invocation.
* **appPort**: int: The Dapr appPort. Specifies the internal listening port for the application to handle requests from the Dapr sidecar.
* **config**: string: Specifies the Dapr configuration to use for the resource.
* **kind**: 'daprSidecar' (Required): Specifies the extensions of a resource.
* **protocol**: 'TCP' | 'UDP' | 'grpc' | 'http': Protocol in use by the port
* **provides**: string: Specifies the resource id of a dapr.io.InvokeHttpRoute that can route traffic to this resource.

### ManualScalingExtension
#### Properties
* **kind**: 'manualScaling' (Required): Specifies the extensions of a resource.
* **replicas**: int: Replica count.


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
* **providers**: [ProviderProperties](#providerproperties): Cloud provider configuration
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the resource at the time the operation was called.
* **recipes**: [EnvironmentPropertiesRecipes](#environmentpropertiesrecipes): Dictionary of <EnvironmentRecipeProperties>

## EnvironmentCompute
* **Discriminator**: kind

### Base Properties
* **resourceId**: string: The resource id of the compute resource for application environment.
### KubernetesCompute
#### Properties
* **kind**: 'kubernetes' (Required): Type of compute resource.
* **namespace**: string (Required): The namespace to use for the environment.


## ProviderProperties
### Properties
* **azure**: [ProviderPropertiesAzure](#providerpropertiesazure): Azure cloud provider configuration

## ProviderPropertiesAzure
### Properties
* **scope**: string: Target scope for Azure resources to be deployed into.  For example: '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup'

## EnvironmentPropertiesRecipes
### Properties
### Additional Properties
* **Additional Properties Type**: [EnvironmentRecipeProperties](#environmentrecipeproperties)

## EnvironmentRecipeProperties
### Properties
* **connectorType**: string (Required): Type of the connector this recipe can be consumed by. For example: 'Applications.Connector/mongoDatabases'
* **templatePath**: string (Required): Path to the template provided by the recipe. Currently only link to Azure Container Registry is supported.

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## GatewayProperties
### Properties
* **application**: string (Required): The resource id of the application linked to Gateway resource.
* **hostname**: [GatewayPropertiesHostname](#gatewaypropertieshostname): Declare hostname information for the Gateway. Leaving the hostname empty auto-assigns one: mygateway.myapp.PUBLICHOSTNAMEORIP.nip.io.
* **internal**: bool: Sets Gateway to not be exposed externally (no public IP address associated). Defaults to false (exposed to internet).
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the resource at the time the operation was called.
* **routes**: [GatewayRoute](#gatewayroute)[] (Required): Routes attached to this Gateway
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **url**: string (ReadOnly): URL of the gateway resource. Readonly.

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
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the resource at the time the operation was called.
* **scheme**: string: The scheme used for traffic. Readonly.
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **url**: string: A stable URL that that can be used to route traffic to a resource. Readonly.

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## VolumeProperties
* **Discriminator**: kind

### Base Properties
* **application**: string: Fully qualified resource ID for the application that the volume is connected to.
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the resource at the time the operation was called.
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
### AzureKeyVaultVolumeProperties
#### Properties
* **certificates**: [AzureKeyVaultVolumePropertiesCertificates](#azurekeyvaultvolumepropertiescertificates): The KeyVault certificates that this volume exposes
* **identity**: [AzureIdentity](#azureidentity) (Required)
* **keys**: [AzureKeyVaultVolumePropertiesKeys](#azurekeyvaultvolumepropertieskeys): The KeyVault keys that this volume exposes
* **kind**: 'azure.com.keyvault' (Required): The volume kind
* **resource**: string (Required): The ID of the keyvault to use for this volume resource
* **secrets**: [AzureKeyVaultVolumePropertiesSecrets](#azurekeyvaultvolumepropertiessecrets): The KeyVault secrets that this volume exposes


## AzureKeyVaultVolumePropertiesCertificates
### Properties
### Additional Properties
* **Additional Properties Type**: [CertificateObjectProperties](#certificateobjectproperties)

## CertificateObjectProperties
### Properties
* **alias**: string: File name when written to disk.
* **certType**: 'certificate' | 'privatekey' | 'publickey': Certificate object type to be downloaded - the certificate itself, private key or public key of the certificate
* **encoding**: 'base64' | 'hex' | 'utf-8': Encoding format. Default utf-8
* **format**: 'pem' | 'pfx': Certificate format. Default pem
* **name**: string (Required): The name of the certificate
* **version**: string: Certificate version

## AzureIdentity
### Properties
* **clientId**: string (Required): The client ID for workload and user assigned managed identity
* **kind**: 'SystemAssigned' | 'Workload' (Required): Identity Kind
* **tenantId**: string: The tenant ID for workload identity.

## AzureKeyVaultVolumePropertiesKeys
### Properties
### Additional Properties
* **Additional Properties Type**: [KeyObjectProperties](#keyobjectproperties)

## KeyObjectProperties
### Properties
* **alias**: string: File name when written to disk.
* **name**: string (Required): The name of the key
* **version**: string: Key version

## AzureKeyVaultVolumePropertiesSecrets
### Properties
### Additional Properties
* **Additional Properties Type**: [SecretObjectProperties](#secretobjectproperties)

## SecretObjectProperties
### Properties
* **alias**: string: File name when written to disk.
* **encoding**: 'base64' | 'hex' | 'utf-8': Encoding format. Default utf-8
* **name**: string (Required): The name of the secret
* **version**: string: Secret version

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

