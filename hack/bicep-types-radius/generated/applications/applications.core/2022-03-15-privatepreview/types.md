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

## ApplicationExtension
* **Discriminator**: kind

### Base Properties

### ApplicationKubernetesMetadataExtension
#### Properties
* **annotations**: [ApplicationKubernetesMetadataExtensionAnnotations](#applicationkubernetesmetadataextensionannotations): Annotations to be applied to the Kubernetes resources output by the resource
* **kind**: 'kubernetesMetadata' (Required): Specifies the extensions of a resource.
* **labels**: [ApplicationKubernetesMetadataExtensionLabels](#applicationkubernetesmetadataextensionlabels): Labels to be applied to the Kubernetes resources output by the resource

### ApplicationKubernetesNamespaceExtension
#### Properties
* **kind**: 'kubernetesNamespace' (Required): Specifies the extensions of a resource.
* **namespace**: string (Required): The Kubernetes namespace to use for this application.


## ApplicationKubernetesMetadataExtensionAnnotations
### Properties
### Additional Properties
* **Additional Properties Type**: string

## ApplicationKubernetesMetadataExtensionLabels
### Properties
### Additional Properties
* **Additional Properties Type**: string

## ApplicationProperties
### Properties
* **environment**: string (Required): The resource id of the environment linked to application.
* **extensions**: [ApplicationExtension](#applicationextension)[]: Extensions spec of the resource
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' | string (ReadOnly): Provisioning state of the application at the time the operation was called.
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of the resource

## AzureKeyVaultVolumePropertiesCertificates
### Properties
### Additional Properties
* **Additional Properties Type**: [CertificateObjectProperties](#certificateobjectproperties)

## AzureKeyVaultVolumePropertiesKeys
### Properties
### Additional Properties
* **Additional Properties Type**: [KeyObjectProperties](#keyobjectproperties)

## AzureKeyVaultVolumePropertiesSecrets
### Properties
### Additional Properties
* **Additional Properties Type**: [SecretObjectProperties](#secretobjectproperties)

## CertificateObjectProperties
### Properties
* **alias**: string: File name when written to disk.
* **certType**: 'certificate' | 'privatekey' | 'publickey': Certificate object type to be downloaded - the certificate itself, private key or public key of the certificate
* **encoding**: 'base64' | 'hex' | 'utf-8': Encoding format. Default utf-8
* **format**: 'pem' | 'pfx': Certificate format. Default pem
* **name**: string (Required): The name of the certificate
* **version**: string: Certificate version

## ConnectionProperties
### Properties
* **disableDefaultEnvVars**: bool
* **iam**: [IamProperties](#iamproperties): The properties of IAM
* **source**: string (Required): The source of the connection

## Container
### Properties
* **args**: string[]: Arguments to the entrypoint. Overrides the container image's CMD
* **command**: string[]: Entrypoint array. Overrides the container image's ENTRYPOINT
* **env**: [ContainerEnv](#containerenv): Dictionary of <string>
* **image**: string (Required): The registry and image to download and run in your container
* **livenessProbe**: [HealthProbeProperties](#healthprobeproperties): Properties for readiness/liveness probe
* **ports**: [ContainerPorts](#containerports): Dictionary of <ContainerPort>
* **readinessProbe**: [HealthProbeProperties](#healthprobeproperties): Properties for readiness/liveness probe
* **volumes**: [ContainerVolumes](#containervolumes): Dictionary of <Volume>
* **workingDir**: string: Working directory for the container

## ContainerEnv
### Properties
### Additional Properties
* **Additional Properties Type**: string

## ContainerExtension
* **Discriminator**: kind

### Base Properties

### DaprSidecarExtension
#### Properties
* **appId**: string (Required): The Dapr appId. Specifies the identifier used by Dapr for service invocation.
* **appPort**: int: The Dapr appPort. Specifies the internal listening port for the application to handle requests from the Dapr sidecar.
* **config**: string: Specifies the Dapr configuration to use for the resource.
* **kind**: 'daprSidecar' (Required): Specifies the extensions of a resource.
* **protocol**: 'TCP' | 'UDP' | 'grpc' | 'http' | string: Specifies the Dapr app-protocol to use for the resource.
* **provides**: string: Specifies the resource id of a dapr.io.InvokeHttpRoute that can route traffic to this resource.

### ContainerKubernetesMetadataExtension
#### Properties
* **annotations**: [ContainerKubernetesMetadataExtensionAnnotations](#containerkubernetesmetadataextensionannotations): Annotations to be applied to the Kubernetes resources output by the resource
* **kind**: 'kubernetesMetadata' (Required): Specifies the extensions of a resource.
* **labels**: [ContainerKubernetesMetadataExtensionLabels](#containerkubernetesmetadataextensionlabels): Labels to be applied to the Kubernetes resources output by the resource

### ManualScalingExtension
#### Properties
* **kind**: 'manualScaling' (Required): Specifies the extensions of a resource.
* **replicas**: int: Replica count.


## ContainerKubernetesMetadataExtensionAnnotations
### Properties
### Additional Properties
* **Additional Properties Type**: string

## ContainerKubernetesMetadataExtensionLabels
### Properties
### Additional Properties
* **Additional Properties Type**: string

## ContainerPort
### Properties
* **containerPort**: int (Required): The listening port number
* **protocol**: 'TCP' | 'UDP' | 'grpc' | 'http' | string: Protocol in use by the port
* **provides**: string: Specifies a route provided by this port

## ContainerPorts
### Properties
### Additional Properties
* **Additional Properties Type**: [ContainerPort](#containerport)

## ContainerProperties
### Properties
* **application**: string (Required): Specifies the resource id of the application
* **connections**: [ContainerPropertiesConnections](#containerpropertiesconnections): Dictionary of <ConnectionProperties>
* **container**: [Container](#container) (Required): Definition of a container.
* **environment**: string: The resource id of the environment linked to the resource
* **extensions**: [ContainerExtension](#containerextension)[]: Extensions spec of the resource
* **identity**: [IdentitySettings](#identitysettings): Configuration for supported external identity providers
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' | string (ReadOnly): Gets the status of the container at the time the operation was called.
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of the resource

## ContainerPropertiesConnections
### Properties
### Additional Properties
* **Additional Properties Type**: [ConnectionProperties](#connectionproperties)

## ContainerVolumes
### Properties
### Additional Properties
* **Additional Properties Type**: [Volume](#volume)

## EnvironmentCompute
* **Discriminator**: kind

### Base Properties
* **identity**: [IdentitySettings](#identitysettings): Configuration for supported external identity providers
* **resourceId**: string: The resource id of the compute resource for application environment.

### KubernetesCompute
#### Properties
* **kind**: 'kubernetes' (Required): Type of compute resource.
* **namespace**: string (Required): The namespace to use for the environment.


## EnvironmentExtension
* **Discriminator**: kind

### Base Properties

### EnvironmentKubernetesMetadataExtension
#### Properties
* **annotations**: [EnvironmentKubernetesMetadataExtensionAnnotations](#environmentkubernetesmetadataextensionannotations): Annotations to be applied to the Kubernetes resources output by the resource
* **kind**: 'kubernetesMetadata' (Required): Specifies the extensions of a resource.
* **labels**: [EnvironmentKubernetesMetadataExtensionLabels](#environmentkubernetesmetadataextensionlabels): Labels to be applied to the Kubernetes resources output by the resource


## EnvironmentKubernetesMetadataExtensionAnnotations
### Properties
### Additional Properties
* **Additional Properties Type**: string

## EnvironmentKubernetesMetadataExtensionLabels
### Properties
### Additional Properties
* **Additional Properties Type**: string

## EnvironmentProperties
### Properties
* **compute**: [EnvironmentCompute](#environmentcompute) (Required): Compute resource used by application environment resource.
* **extensions**: [EnvironmentExtension](#environmentextension)[]: Extensions spec of the resource
* **providers**: [Providers](#providers): Cloud providers configuration for the environment.
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' | string (ReadOnly): Provisioning state of the environment at the time the operation was called.
* **recipes**: [EnvironmentPropertiesRecipes](#environmentpropertiesrecipes): Dictionary of <EnvironmentRecipeProperties>
* **useDevRecipes**: bool: Flag to use radius owned recipes.

## EnvironmentPropertiesRecipes
### Properties
### Additional Properties
* **Additional Properties Type**: [EnvironmentRecipeProperties](#environmentrecipeproperties)

## EnvironmentRecipeProperties
### Properties
* **linkType**: string (Required): Type of the link this recipe can be consumed by. For example: 'Applications.Link/mongoDatabases'
* **parameters**: any: Key/value parameters to pass to the recipe template at deployment
* **templatePath**: string (Required): Path to the template provided by the recipe. Currently only link to Azure Container Registry is supported.

## GatewayProperties
### Properties
* **application**: string (Required): Specifies the resource id of the application
* **environment**: string: The resource id of the environment linked to the resource
* **hostname**: [GatewayPropertiesHostname](#gatewaypropertieshostname): Declare hostname information for the Gateway. Leaving the hostname empty auto-assigns one: mygateway.myapp.PUBLICHOSTNAMEORIP.nip.io.
* **internal**: bool: Sets Gateway to not be exposed externally (no public IP address associated). Defaults to false (exposed to internet).
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' | string (ReadOnly): Provisioning state of the Gateway at the time the operation was called.
* **routes**: [GatewayRoute](#gatewayroute)[] (Required): Routes attached to this Gateway
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of the resource
* **tls**: [GatewayPropertiesTls](#gatewaypropertiestls): TLS configuration for the Gateway.
* **url**: string (ReadOnly): URL of the gateway resource. Readonly.

## GatewayPropertiesHostname
### Properties
* **fullyQualifiedHostname**: string: Specify a fully-qualified domain name: myapp.mydomain.com. Mutually exclusive with 'prefix' and will take priority if both are defined.
* **prefix**: string: Specify a prefix for the hostname: myhostname.myapp.PUBLICHOSTNAMEORIP.nip.io. Mutually exclusive with 'fullyQualifiedHostname' and will be overridden if both are defined.

## GatewayPropertiesTls
### Properties
* **sslPassthrough**: bool: If true, gateway lets the https traffic sslPassthrough to the backend servers for decryption.

## GatewayRoute
### Properties
* **destination**: string: The HttpRoute to route to. Ex - myserviceroute.id.
* **path**: string: The path to match the incoming request path on. Ex - /myservice.
* **replacePrefix**: string: Optionally update the prefix when sending the request to the service. Ex - replacePrefix: '/' and path: '/myservice' will transform '/myservice/myroute' to '/myroute'

## HealthProbeProperties
* **Discriminator**: kind

### Base Properties
* **failureThreshold**: int: Threshold number of times the probe fails after which a failure would be reported
* **initialDelaySeconds**: int: Initial delay in seconds before probing for readiness/liveness
* **periodSeconds**: int: Interval for the readiness/liveness probe in seconds
* **timeoutSeconds**: int: Number of seconds after which the readiness/liveness probe times out. Defaults to 5 seconds

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

## HttpRouteProperties
### Properties
* **application**: string (Required): Specifies the resource id of the application
* **environment**: string: The resource id of the environment linked to the resource
* **hostname**: string: The internal hostname accepting traffic for the HTTP Route. Readonly.
* **port**: int: The port number for the HTTP Route. Defaults to 80. Readonly.
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' | string (ReadOnly): Provisioning state of the HTTP Route at the time the operation was called.
* **scheme**: string: The scheme used for traffic. Readonly.
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of the resource
* **url**: string: A stable URL that that can be used to route traffic to a resource. Readonly.

## IamProperties
### Properties
* **kind**: 'azure' | string (Required): The kind of IAM provider to configure
* **roles**: string[]: RBAC permissions to be assigned on the source resource

## IdentitySettings
### Properties
* **kind**: 'azure.com.workload' | 'undefined' (Required): Configuration for supported external identity providers
* **oidcIssuer**: string: The URI for your compute platform's OIDC issuer
* **resource**: string: The resource ID of the provisioned identity

## KeyObjectProperties
### Properties
* **alias**: string: File name when written to disk.
* **name**: string (Required): The name of the key
* **version**: string: Key version

## Providers
### Properties
* **aws**: [ProvidersAws](#providersaws): AWS cloud provider configuration
* **azure**: [ProvidersAzure](#providersazure): Azure cloud provider configuration

## ProvidersAws
### Properties
* **scope**: string: Target scope for AWS resources to be deployed into.  For example: '/planes/aws/aws/accounts/000000000000/regions/us-west-2'

## ProvidersAzure
### Properties
* **scope**: string: Target scope for Azure resources to be deployed into.  For example: '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup'

## ResourceStatus
### Properties
* **compute**: [EnvironmentCompute](#environmentcompute): Compute resource used by application environment resource.
* **outputResources**: any[]

## SecretObjectProperties
### Properties
* **alias**: string: File name when written to disk.
* **encoding**: 'base64' | 'hex' | 'utf-8': Encoding format. Default utf-8
* **name**: string (Required): The name of the secret
* **version**: string: Secret version

## SystemData
### Properties
* **createdAt**: string: The timestamp of resource creation (UTC).
* **createdBy**: string: The identity that created the resource.
* **createdByType**: 'Application' | 'Key' | 'ManagedIdentity' | 'User' | string: The type of identity that created the resource.
* **lastModifiedAt**: string: The timestamp of resource last modification (UTC)
* **lastModifiedBy**: string: The identity that last modified the resource.
* **lastModifiedByType**: 'Application' | 'Key' | 'ManagedIdentity' | 'User' | string: The type of identity that last modified the resource.

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## Volume
* **Discriminator**: kind

### Base Properties
* **mountPath**: string: The path where the volume is mounted

### EphemeralVolume
#### Properties
* **kind**: 'ephemeral' (Required): The Volume kind
* **managedStore**: 'disk' | 'memory' | string (Required): Backing store for the ephemeral volume

### PersistentVolume
#### Properties
* **kind**: 'persistent' (Required): The Volume kind
* **permission**: 'read' | 'write' | string: Container read/write access to the volume
* **source**: string (Required): The source of the volume


## VolumeProperties
* **Discriminator**: kind

### Base Properties
* **application**: string (Required): Specifies the resource id of the application
* **environment**: string: The resource id of the environment linked to the resource
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' | string (ReadOnly): Provisioning state of the Volume at the time the operation was called.
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of the resource

### AzureKeyVaultVolumeProperties
#### Properties
* **certificates**: [AzureKeyVaultVolumePropertiesCertificates](#azurekeyvaultvolumepropertiescertificates): The KeyVault certificates that this volume exposes
* **keys**: [AzureKeyVaultVolumePropertiesKeys](#azurekeyvaultvolumepropertieskeys): The KeyVault keys that this volume exposes
* **kind**: 'azure.com.keyvault' (Required): The volume kind
* **resource**: string (Required): The ID of the keyvault to use for this volume resource
* **secrets**: [AzureKeyVaultVolumePropertiesSecrets](#azurekeyvaultvolumepropertiessecrets): The KeyVault secrets that this volume exposes


