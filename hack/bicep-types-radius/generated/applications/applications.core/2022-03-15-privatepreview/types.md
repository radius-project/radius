# Applications.Core @ 2022-03-15-privatepreview

## Resource Applications.Core/applications@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [ApplicationProperties](#applicationproperties): Application properties
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
* **properties**: [ContainerProperties](#containerproperties): Container properties
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
* **properties**: [EnvironmentProperties](#environmentproperties): Environment properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Core/environments' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Core/extenders@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [ExtenderProperties](#extenderproperties): ExtenderResource link properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Core/extenders' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Core/gateways@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [GatewayProperties](#gatewayproperties): Gateway properties
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
* **properties**: [HttpRouteProperties](#httprouteproperties): HTTPRoute properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Core/httpRoutes' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Core/secretStores@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [SecretStoreProperties](#secretstoreproperties): SecretStore properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Core/secretStores' (ReadOnly, DeployTimeConstant): The resource type

## Resource Applications.Core/volumes@2022-03-15-privatepreview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2022-03-15-privatepreview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [VolumeProperties](#volumeproperties): Volume properties
* **systemData**: [SystemData](#systemdata) (ReadOnly): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Core/volumes' (ReadOnly, DeployTimeConstant): The resource type

## Function listSecrets (Applications.Core/extenders@2022-03-15-privatepreview)
* **Resource**: Applications.Core/extenders
* **ApiVersion**: 2022-03-15-privatepreview
* **Input**: any
* **Output**: any

## Function listSecrets (Applications.Core/secretStores@2022-03-15-privatepreview)
* **Resource**: Applications.Core/secretStores
* **ApiVersion**: 2022-03-15-privatepreview
* **Input**: any
* **Output**: [SecretStoreListSecretsResult](#secretstorelistsecretsresult)

## ApplicationProperties
### Properties
* **environment**: string (Required): Fully qualified resource ID for the environment that the portable resource is linked to
* **extensions**: [Extension](#extension)[] (Required): The application extension.
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the portable resource at the time the operation was called
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.

## Extension
* **Discriminator**: kind

### Base Properties
### DaprSidecarExtension
#### Properties
* **appId**: string (Required): The Dapr appId. Specifies the identifier used by Dapr for service invocation.
* **appPort**: int: The Dapr appPort. Specifies the internal listening port for the application to handle requests from the Dapr sidecar.
* **config**: string: Specifies the Dapr configuration to use for the resource.
* **kind**: 'daprSidecar' (Required): Discriminator property for Extension.
* **protocol**: 'grpc' | 'http': The Dapr sidecar extension protocol

### KubernetesMetadataExtension
#### Properties
* **annotations**: [KubernetesMetadataExtensionAnnotations](#kubernetesmetadataextensionannotations) (Required): Annotations to be applied to the Kubernetes resources output by the resource
* **kind**: 'kubernetesMetadata' (Required): Discriminator property for Extension.
* **labels**: [KubernetesMetadataExtensionLabels](#kubernetesmetadataextensionlabels) (Required): Labels to be applied to the Kubernetes resources output by the resource

### KubernetesNamespaceExtension
#### Properties
* **kind**: 'kubernetesNamespace' (Required): Discriminator property for Extension.
* **namespace**: string (Required): The namespace of the application environment.

### ManualScalingExtension
#### Properties
* **kind**: 'manualScaling' (Required): Discriminator property for Extension.
* **replicas**: int (Required): Replica count.


## KubernetesMetadataExtensionAnnotations
### Properties
### Additional Properties
* **Additional Properties Type**: string

## KubernetesMetadataExtensionLabels
### Properties
### Additional Properties
* **Additional Properties Type**: string

## ResourceStatus
### Properties
* **compute**: [EnvironmentCompute](#environmentcompute): Represents backing compute resource
* **outputResources**: [OutputResource](#outputresource)[]: Properties of an output resource

## EnvironmentCompute
* **Discriminator**: kind

### Base Properties
* **identity**: [IdentitySettings](#identitysettings): IdentitySettings is the external identity setting.
* **resourceId**: string: The resource id of the compute resource for application environment.
### KubernetesCompute
#### Properties
* **kind**: 'kubernetes' (Required): Discriminator property for EnvironmentCompute.
* **namespace**: string (Required): The namespace to use for the environment.


## IdentitySettings
### Properties
* **kind**: 'azure.com.workload' | 'undefined' (Required): IdentitySettingKind is the kind of supported external identity setting
* **oidcIssuer**: string: The URI for your compute platform's OIDC issuer
* **resource**: string: The resource ID of the provisioned identity

## OutputResource
### Properties
* **id**: string: The UCP resource ID of the underlying resource.
* **localId**: string: The logical identifier scoped to the owning Radius resource. This is only needed or used when a resource has a dependency relationship. LocalIDs do not have any particular format or meaning beyond being compared to determine dependency relationships.
* **radiusManaged**: bool: Determines whether Radius manages the lifecycle of the underlying resource.

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
* **application**: string (Required): Fully qualified resource ID for the application that the portable resource is consumed by
* **connections**: [ContainerPropertiesConnections](#containerpropertiesconnections): Specifies a connection to another resource.
* **container**: [Container](#container) (Required): Definition of a container
* **environment**: string: Fully qualified resource ID for the environment that the portable resource is linked to (if applicable)
* **extensions**: [Extension](#extension)[]: Extensions spec of the resource
* **identity**: [IdentitySettings](#identitysettings): IdentitySettings is the external identity setting.
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the portable resource at the time the operation was called
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.

## ContainerPropertiesConnections
### Properties
### Additional Properties
* **Additional Properties Type**: [ConnectionProperties](#connectionproperties)

## ConnectionProperties
### Properties
* **disableDefaultEnvVars**: bool: default environment variable override
* **iam**: [IamProperties](#iamproperties): IAM properties
* **source**: string (Required): The source of the connection

## IamProperties
### Properties
* **kind**: 'azure' (Required): The kind of IAM provider to configure
* **roles**: string[]: RBAC permissions to be assigned on the source resource

## Container
### Properties
* **args**: string[]: Arguments to the entrypoint. Overrides the container image's CMD
* **command**: string[]: Entrypoint array. Overrides the container image's ENTRYPOINT
* **env**: [ContainerEnv](#containerenv): environment
* **image**: string (Required): The registry and image to download and run in your container
* **livenessProbe**: [HealthProbeProperties](#healthprobeproperties): Properties for readiness/liveness probe
* **ports**: [ContainerPorts](#containerports): container ports
* **readinessProbe**: [HealthProbeProperties](#healthprobeproperties): Properties for readiness/liveness probe
* **volumes**: [ContainerVolumes](#containervolumes): container volumes
* **workingDir**: string (Required): Working directory for the container

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
* **timeoutSeconds**: int: Number of seconds after which the readiness/liveness probe times out. Defaults to 5 seconds
### ExecHealthProbeProperties
#### Properties
* **command**: string (Required): Command to execute to probe readiness/liveness
* **kind**: 'exec' (Required): Discriminator property for HealthProbeProperties.

### HttpGetHealthProbeProperties
#### Properties
* **containerPort**: int (Required): The listening port number
* **headers**: [HttpGetHealthProbePropertiesHeaders](#httpgethealthprobepropertiesheaders): Custom HTTP headers to add to the get request
* **kind**: 'httpGet' (Required): Discriminator property for HealthProbeProperties.
* **path**: string (Required): The route to make the HTTP request on

### TcpHealthProbeProperties
#### Properties
* **containerPort**: int (Required): The listening port number
* **kind**: 'tcp' (Required): Discriminator property for HealthProbeProperties.


## HttpGetHealthProbePropertiesHeaders
### Properties
### Additional Properties
* **Additional Properties Type**: string

## ContainerPorts
### Properties
### Additional Properties
* **Additional Properties Type**: [ContainerPortProperties](#containerportproperties)

## ContainerPortProperties
### Properties
* **containerPort**: int (Required): The listening port number
* **port**: int: Specifies the port that will be exposed by this container. Must be set when value different from containerPort is desired
* **protocol**: 'TCP' | 'UDP': The protocol in use by the port
* **provides**: string: Specifies a route provided by this port
* **scheme**: string: Specifies the URL scheme of the communication protocol. Consumers can use the scheme to construct a URL. The value defaults to 'http' or 'https' depending on the port value

## ContainerVolumes
### Properties
### Additional Properties
* **Additional Properties Type**: [Volume](#volume)

## Volume
* **Discriminator**: kind

### Base Properties
* **mountPath**: string (Required): The path where the volume is mounted
### EphemeralVolume
#### Properties
* **kind**: 'ephemeral' (Required): Discriminator property for Volume.
* **managedStore**: 'disk' | 'memory' (Required): The managed store for the ephemeral volume

### PersistentVolume
#### Properties
* **kind**: 'persistent' (Required): Discriminator property for Volume.
* **permission**: 'read' | 'write': The persistent volume permission
* **source**: string (Required): The source of the volume


## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## EnvironmentProperties
### Properties
* **compute**: [EnvironmentCompute](#environmentcompute) (Required): Represents backing compute resource
* **extensions**: [Extension](#extension)[]: The environment extension.
* **providers**: [Providers](#providers): The Cloud providers configuration
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the portable resource at the time the operation was called
* **recipes**: [EnvironmentPropertiesRecipes](#environmentpropertiesrecipes): Specifies Recipes linked to the Environment.

## Providers
### Properties
* **aws**: [ProvidersAws](#providersaws): The AWS cloud provider definition
* **azure**: [ProvidersAzure](#providersazure): The Azure cloud provider definition

## ProvidersAws
### Properties
* **scope**: string (Required): Target scope for AWS resources to be deployed into.  For example: '/planes/aws/aws/accounts/000000000000/regions/us-west-2'

## ProvidersAzure
### Properties
* **scope**: string (Required): Target scope for Azure resources to be deployed into.  For example: '/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup'

## EnvironmentPropertiesRecipes
### Properties
### Additional Properties
* **Additional Properties Type**: [DictionaryOfRecipeProperties](#dictionaryofrecipeproperties)

## DictionaryOfRecipeProperties
### Properties
### Additional Properties
* **Additional Properties Type**: [RecipeProperties](#recipeproperties)

## RecipeProperties
* **Discriminator**: templateKind

### Base Properties
* **parameters**: any: Any object
* **templatePath**: string (Required): Path to the template provided by the recipe. Currently only link to Azure Container Registry is supported.
### BicepRecipeProperties
#### Properties
* **templateKind**: 'bicep' (Required): Discriminator property for RecipeProperties.

### TerraformRecipeProperties
#### Properties
* **templateKind**: 'terraform' (Required): Discriminator property for RecipeProperties.
* **templateVersion**: string (Required): Version of the template to deploy. For Terraform recipes using a module registry this is required, but must be omitted for other module sources.


## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## ExtenderProperties
### Properties
* **application**: string: Fully qualified resource ID for the application that the portable resource is consumed by (if applicable)
* **environment**: string (Required): Fully qualified resource ID for the environment that the portable resource is linked to
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the portable resource at the time the operation was called
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a link
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **secrets**: any: Any object
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
### Additional Properties
* **Additional Properties Type**: any

## Recipe
### Properties
* **name**: string (Required): The name of the recipe within the environment to use
* **parameters**: any: Any object

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## GatewayProperties
### Properties
* **application**: string (Required): Fully qualified resource ID for the application that the portable resource is consumed by
* **environment**: string: Fully qualified resource ID for the environment that the portable resource is linked to (if applicable)
* **hostname**: [GatewayHostname](#gatewayhostname): Declare hostname information for the Gateway. Leaving the hostname empty auto-assigns one: mygateway.myapp.PUBLICHOSTNAMEORIP.nip.io.
* **internal**: bool: Sets Gateway to not be exposed externally (no public IP address associated). Defaults to false (exposed to internet).
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the portable resource at the time the operation was called
* **routes**: [GatewayRoute](#gatewayroute)[] (Required): Routes attached to this Gateway
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **tls**: [GatewayTls](#gatewaytls): TLS configuration definition for Gateway resource.
* **url**: string (ReadOnly): URL of the gateway resource. Readonly

## GatewayHostname
### Properties
* **fullyQualifiedHostname**: string: Specify a fully-qualified domain name: myapp.mydomain.com. Mutually exclusive with 'prefix' and will take priority if both are defined.
* **prefix**: string: Specify a prefix for the hostname: myhostname.myapp.PUBLICHOSTNAMEORIP.nip.io. Mutually exclusive with 'fullyQualifiedHostname' and will be overridden if both are defined.

## GatewayRoute
### Properties
* **destination**: string (Required): The HttpRoute to route to. Ex - myserviceroute.id.
* **path**: string (Required): The path to match the incoming request path on. Ex - /myservice.
* **replacePrefix**: string: Optionally update the prefix when sending the request to the service. Ex - replacePrefix: '/' and path: '/myservice' will transform '/myservice/myroute' to '/myroute'

## GatewayTls
### Properties
* **certificateFrom**: string: The resource id for the secret containing the TLS certificate and key for the gateway.
* **minimumProtocolVersion**: '1.2' | '1.3': Tls Minimum versions for Gateway resource.
* **sslPassthrough**: bool: If true, gateway lets the https traffic sslPassthrough to the backend servers for decryption.

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## HttpRouteProperties
### Properties
* **application**: string (Required): Fully qualified resource ID for the application that the portable resource is consumed by
* **environment**: string: Fully qualified resource ID for the environment that the portable resource is linked to (if applicable)
* **hostname**: string: The internal hostname accepting traffic for the HTTP Route. Readonly.
* **port**: int: The port number for the HTTP Route. Defaults to 80. Readonly.
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the portable resource at the time the operation was called
* **scheme**: string (ReadOnly): The scheme used for traffic. Readonly.
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **url**: string (ReadOnly): A stable URL that that can be used to route traffic to a resource. Readonly.

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## SecretStoreProperties
### Properties
* **application**: string (Required): Fully qualified resource ID for the application that the portable resource is consumed by
* **data**: [SecretStorePropertiesData](#secretstorepropertiesdata) (Required): An object to represent key-value type secrets
* **environment**: string: Fully qualified resource ID for the environment that the portable resource is linked to (if applicable)
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the portable resource at the time the operation was called
* **resource**: string: The resource id of external secret store.
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
* **type**: 'certificate' | 'generic': SecretStore data type

## SecretStorePropertiesData
### Properties
### Additional Properties
* **Additional Properties Type**: [SecretValueProperties](#secretvalueproperties)

## SecretValueProperties
### Properties
* **encoding**: 'base64' | 'raw': SecretValue Encoding type
* **value**: string (Required): The value of secret.
* **valueFrom**: [ValueFromProperties](#valuefromproperties): The Secret value source properties

## ValueFromProperties
### Properties
* **name**: string (Required): The name of the referenced secret.
* **version**: string: The version of the referenced secret.

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## VolumeProperties
* **Discriminator**: kind

### Base Properties
* **application**: string (Required): Fully qualified resource ID for the application that the portable resource is consumed by
* **environment**: string: Fully qualified resource ID for the environment that the portable resource is linked to (if applicable)
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (ReadOnly): Provisioning state of the portable resource at the time the operation was called
* **status**: [ResourceStatus](#resourcestatus) (ReadOnly): Status of a resource.
### AzureKeyVaultVolumeProperties
#### Properties
* **certificates**: [AzureKeyVaultVolumePropertiesCertificates](#azurekeyvaultvolumepropertiescertificates): The KeyVault certificates that this volume exposes
* **keys**: [AzureKeyVaultVolumePropertiesKeys](#azurekeyvaultvolumepropertieskeys): The KeyVault keys that this volume exposes
* **kind**: 'azure.com.keyvault' (Required): Discriminator property for VolumeProperties.
* **resource**: string (Required): The ID of the keyvault to use for this volume resource
* **secrets**: [AzureKeyVaultVolumePropertiesSecrets](#azurekeyvaultvolumepropertiessecrets): The KeyVault secrets that this volume exposes


## AzureKeyVaultVolumePropertiesCertificates
### Properties
### Additional Properties
* **Additional Properties Type**: [CertificateObjectProperties](#certificateobjectproperties)

## CertificateObjectProperties
### Properties
* **alias**: string: File name when written to disk
* **certType**: 'certificate' | 'privatekey' | 'publickey': Represents certificate types
* **encoding**: 'base64' | 'hex' | 'utf-8': Represents secret encodings
* **format**: 'pem' | 'pfx': Represents certificate formats
* **name**: string (Required): The name of the certificate
* **version**: string: Certificate version

## AzureKeyVaultVolumePropertiesKeys
### Properties
### Additional Properties
* **Additional Properties Type**: [KeyObjectProperties](#keyobjectproperties)

## KeyObjectProperties
### Properties
* **alias**: string: File name when written to disk
* **name**: string (Required): The name of the certificate
* **version**: string: Certificate version

## AzureKeyVaultVolumePropertiesSecrets
### Properties
### Additional Properties
* **Additional Properties Type**: [SecretObjectProperties](#secretobjectproperties)

## SecretObjectProperties
### Properties
* **alias**: string: File name when written to disk
* **encoding**: 'base64' | 'hex' | 'utf-8': Represents secret encodings
* **name**: string (Required): The name of the certificate
* **version**: string: Certificate version

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## SecretStoreListSecretsResult
### Properties
* **data**: [SecretStoreListSecretsResultData](#secretstorelistsecretsresultdata) (ReadOnly): An object to represent key-value type secrets
* **type**: 'certificate' | 'generic' (ReadOnly): SecretStore data type

## SecretStoreListSecretsResultData
### Properties
### Additional Properties
* **Additional Properties Type**: [SecretValueProperties](#secretvalueproperties)

