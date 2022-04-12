# Microsoft.CustomProviders @ 2018-09-01-preview

## Resource Microsoft.CustomProviders/resourceProviders/Application@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string: The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [ApplicationProperties](#applicationproperties): Properties of an application.
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application' (ReadOnly, DeployTimeConstant): The resource type

## Resource Microsoft.CustomProviders/resourceProviders/Application/AzureConnection@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: any (Required): Any object
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application/AzureConnection' (ReadOnly, DeployTimeConstant): The resource type

## Resource Microsoft.CustomProviders/resourceProviders/Application/Container@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [ContainerProperties](#containerproperties) (Required)
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application/Container' (ReadOnly, DeployTimeConstant): The resource type

## Resource Microsoft.CustomProviders/resourceProviders/Application/dapr.io.InvokeHttpRoute@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprHttpRouteProperties](#daprhttprouteproperties) (Required)
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application/dapr.io.InvokeHttpRoute' (ReadOnly, DeployTimeConstant): The resource type

## Resource Microsoft.CustomProviders/resourceProviders/Application/dapr.io.PubSubTopic@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprPubSubTopicProperties](#daprpubsubtopicproperties) (Required)
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application/dapr.io.PubSubTopic' (ReadOnly, DeployTimeConstant): The resource type

## Resource Microsoft.CustomProviders/resourceProviders/Application/dapr.io.SecretStore@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprSecretStoreProperties](#daprsecretstoreproperties) (Required)
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application/dapr.io.SecretStore' (ReadOnly, DeployTimeConstant): The resource type

## Resource Microsoft.CustomProviders/resourceProviders/Application/dapr.io.StateStore@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [DaprStateStoreResourceProperties](#daprstatestoreresourceproperties) (Required)
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application/dapr.io.StateStore' (ReadOnly, DeployTimeConstant): The resource type

## Resource Microsoft.CustomProviders/resourceProviders/Application/Extender@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [ExtenderProperties](#extenderproperties) (Required)
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application/Extender' (ReadOnly, DeployTimeConstant): The resource type

## Resource Microsoft.CustomProviders/resourceProviders/Application/Gateway@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [GatewayProperties](#gatewayproperties)
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application/Gateway' (ReadOnly, DeployTimeConstant): The resource type

## Resource Microsoft.CustomProviders/resourceProviders/Application/HttpRoute@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [HttpRouteProperties](#httprouteproperties)
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application/HttpRoute' (ReadOnly, DeployTimeConstant): The resource type

## Resource Microsoft.CustomProviders/resourceProviders/Application/microsoft.com.SQLDatabase@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [MicrosoftSQLDatabaseProperties](#microsoftsqldatabaseproperties) (Required)
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application/microsoft.com.SQLDatabase' (ReadOnly, DeployTimeConstant): The resource type

## Resource Microsoft.CustomProviders/resourceProviders/Application/mongo.com.MongoDatabase@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [MongoDBResourceProperties](#mongodbresourceproperties)
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application/mongo.com.MongoDatabase' (ReadOnly, DeployTimeConstant): The resource type

## Resource Microsoft.CustomProviders/resourceProviders/Application/rabbitmq.com.MessageQueue@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [RabbitMQMessageQueueResourceProperties](#rabbitmqmessagequeueresourceproperties) (Required)
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application/rabbitmq.com.MessageQueue' (ReadOnly, DeployTimeConstant): The resource type

## Resource Microsoft.CustomProviders/resourceProviders/Application/redislabs.com.RedisCache@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [RedisCacheResourceProperties](#rediscacheresourceproperties) (Required)
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application/redislabs.com.RedisCache' (ReadOnly, DeployTimeConstant): The resource type

## Resource Microsoft.CustomProviders/resourceProviders/Application/Volume@2018-09-01-preview
* **Valid Scope(s)**: ResourceGroup
### Properties
* **apiVersion**: '2018-09-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [VolumeProperties](#volumeproperties) (Required)
* **status**: [ResourceStatus](#resourcestatus): Status of an application.
* **type**: 'Microsoft.CustomProviders/resourceProviders/Application/Volume' (ReadOnly, DeployTimeConstant): The resource type

## ApplicationProperties
### Properties
* **status**: [ApplicationStatus](#applicationstatus): Status of an application.

## ApplicationStatus
### Properties
* **healthErrorDetails**: string: Health errors for the application
* **healthState**: string: Health state of the application
* **provisioningErrorDetails**: string: Provisioning errors for the application
* **provisioningState**: string: Provisioning state of the application

## ResourceStatus
### Properties
* **healthErrorDetails**: string: Health errors for the application
* **healthState**: string: Health state of the application
* **provisioningErrorDetails**: string: Provisioning errors for the application
* **provisioningState**: string: Provisioning state of the application

## TrackedResourceTags
### Properties
### Additional Properties
* **Additional Properties Type**: string

## ContainerProperties
### Properties
* **connections**: [ContainerPropertiesConnections](#containerpropertiesconnections): Dictionary of <Connection>
* **container**: [Container](#container): Definition of a container.
* **status**: [ResourceStatusAutoGenerated](#resourcestatusautogenerated): Status of a resource.
* **traits**: [ResourceTrait](#resourcetrait)[]: Traits spec of the resource

## ContainerPropertiesConnections
### Properties
### Additional Properties
* **Additional Properties Type**: [Connection](#connection)

## Connection
### Properties
* **kind**: 'Grpc' | 'Http' | 'azure' | 'azure.com/KeyVault' | 'azure.com/ServiceBusQueue' | 'dapr.io/InvokeHttp' | 'dapr.io/PubSubTopic' | 'dapr.io/SecretStore' | 'dapr.io/StateStore' | 'microsoft.com/SQL' | 'mongo.com/MongoDB' | 'rabbitmq.com/MessageQueue' | 'redislabs.com/Redis' (Required): The kind of connection
* **roles**: string[]: RBAC permissions to be assigned on the source resource
* **source**: string (Required): The source of the connection

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
### ExecHealthProbeProperties
#### Properties
* **command**: string (Required): Command to execute to probe readiness/liveness
* **failureThreshold**: int: Threshold number of times the probe fails after which a failure would be reported
* **initialDelaySeconds**: int: Initial delay in seconds before probing for readiness/liveness
* **kind**: 'exec' (Required): The HealthProbeProperties kind
* **periodSeconds**: int: Interval for the readiness/liveness probe in seconds

### HttpGetHealthProbeProperties
#### Properties
* **containerPort**: int (Required): The listening port number
* **failureThreshold**: int: Threshold number of times the probe fails after which a failure would be reported
* **headers**: [HttpGetHealthProbePropertiesHeaders](#httpgethealthprobepropertiesheaders): Custom HTTP headers to add to the get request
* **initialDelaySeconds**: int: Initial delay in seconds before probing for readiness/liveness
* **kind**: 'httpGet' (Required): The HealthProbeProperties kind
* **path**: string (Required): The route to make the HTTP request on
* **periodSeconds**: int: Interval for the readiness/liveness probe in seconds

### TcpHealthProbeProperties
#### Properties
* **containerPort**: int (Required): The listening port number
* **failureThreshold**: int: Threshold number of times the probe fails after which a failure would be reported
* **initialDelaySeconds**: int: Initial delay in seconds before probing for readiness/liveness
* **kind**: 'tcp' (Required): The HealthProbeProperties kind
* **periodSeconds**: int: Interval for the readiness/liveness probe in seconds


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
* **protocol**: 'TCP' | 'UDP': Protocol in use by the port
* **provides**: string: Specifies a route provided by this port

## ContainerVolumes
### Properties
### Additional Properties
* **Additional Properties Type**: [Volume](#volume)

## Volume
* **Discriminator**: kind

### Base Properties
### EphemeralVolume
#### Properties
* **kind**: 'ephemeral' (Required): The Volume kind
* **managedStore**: 'disk' | 'memory' (Required): Backing store for the ephemeral volume
* **mountPath**: string (Required): The path where the volume is mounted

### PersistentVolume
#### Properties
* **kind**: 'persistent' (Required): The Volume kind
* **mountPath**: string (Required): The path where the volume is mounted
* **rbac**: 'read' | 'write': Container read/write access to the volume
* **source**: string (Required): The source of the volume


## ResourceStatusAutoGenerated
### Properties
* **healthErrorDetails**: string: Health errors for the resource
* **healthState**: string: Health state of the resource
* **outputResources**: any[]: Array of AnyObject
* **provisioningErrorDetails**: string: Provisioning errors for the resource
* **provisioningState**: string: Provisioning state of the resource

## ResourceTrait
* **Discriminator**: kind

### Base Properties
### DaprSidecarTrait
#### Properties
* **appId**: string (Required): The Dapr appId. Specifies the identifier used by Dapr for service invocation.
* **appPort**: int: The Dapr appPort. Specifies the internal listening port for the application to handle requests from the Dapr sidecar.
* **config**: string: Specifies the Dapr configuration to use for the resource.
* **kind**: 'dapr.io/Sidecar@v1alpha1' (Required): The ResourceTrait kind
* **protocol**: 'grpc' | 'http': Specifies the Dapr app-protocol to use for the resource.
* **provides**: string: Specifies the resource id of a dapr.io.InvokeHttpRoute that can route traffic to this resource.

### ManualScalingTrait
#### Properties
* **kind**: 'radius.dev/ManualScaling@v1alpha1' (Required): The ResourceTrait kind
* **replicas**: int: Replica count.


## DaprHttpRouteProperties
### Properties
* **appId**: string (Required): The Dapr appId used for the route
* **status**: [RouteStatus](#routestatus): Status of a route.

## RouteStatus
### Properties
* **healthState**: string: Health state of the route
* **outputResources**: any[]: Array of AnyObject
* **provisioningState**: string: Provisioning state of the route

## DaprPubSubTopicProperties
* **Discriminator**: kind

### Base Properties
* **status**: [ResourceStatusAutoGenerated](#resourcestatusautogenerated): Status of a resource.
### DaprPubSubTopicGenericResourceProperties
#### Properties
* **kind**: 'generic' (Required): The DaprPubSubTopicProperties kind
* **metadata**: any (Required): Any object
* **type**: string (Required): Dapr PubSub type. These strings match the format used by Dapr Kubernetes configuration format.
* **version**: string (Required): Dapr component version

### DaprPubSubTopicAzureServiceBusResourceProperties
#### Properties
* **kind**: 'pubsub.azure.servicebus' (Required): The DaprPubSubTopicProperties kind
* **resource**: string (Required): PubSub resource


## DaprSecretStoreProperties
### Properties
* **kind**: 'generic' (Required): Radius kind for Dapr Secret Store
* **metadata**: any (Required): Any object
* **status**: [ResourceStatusAutoGenerated](#resourcestatusautogenerated): Status of a resource.
* **type**: string (Required): Dapr Secret Store type. These strings match the types defined in Dapr Component format: https://docs.dapr.io/reference/components-reference/supported-secret-stores/
* **version**: string (Required): Dapr component version

## DaprStateStoreResourceProperties
* **Discriminator**: kind

### Base Properties
* **status**: [ResourceStatusAutoGenerated](#resourcestatusautogenerated): Status of a resource.
### DaprStateStoreGenericResourceProperties
#### Properties
* **kind**: 'generic' (Required): The DaprStateStoreResourceProperties kind
* **metadata**: any (Required): Any object
* **type**: string (Required): Dapr StateStore type. These strings match the format used by Dapr Kubernetes configuration format.
* **version**: string (Required): Dapr component version

### DaprStateStoreAzureTableStorageResourceProperties
#### Properties
* **kind**: 'state.azure.tablestorage' (Required): The DaprStateStoreResourceProperties kind
* **resource**: string (Required): PubSub resource

### DaprStateStoreSqlServerResourceProperties
#### Properties
* **kind**: 'state.sqlserver' (Required): The DaprStateStoreResourceProperties kind
* **resource**: string (Required): PubSub resource


## ExtenderProperties
### Properties
* **secrets**: [ExtenderPropertiesSecrets](#extenderpropertiessecrets): Dictionary of <any>
* **status**: [ResourceStatusAutoGenerated](#resourcestatusautogenerated): Status of a resource.
### Additional Properties
* **Additional Properties Type**: any

## ExtenderPropertiesSecrets
### Properties
### Additional Properties
* **Additional Properties Type**: any

## GatewayProperties
### Properties
* **listeners**: [GatewayPropertiesListeners](#gatewaypropertieslisteners): Dictionary of <GatewayListener>

## GatewayPropertiesListeners
### Properties
### Additional Properties
* **Additional Properties Type**: [GatewayListener](#gatewaylistener)

## GatewayListener
### Properties
* **port**: int: The port to listen on.
* **protocol**: string: The protocol to use for this listener.

## HttpRouteProperties
### Properties
* **gateway**: [HttpRouteGateway](#httproutegateway): Specifies configuration to allow public traffic from outside the network to the route. Configure a gateway to accept traffic from the internet.
* **hostname**: int: The internal hostname accepting traffic for the route. Readonly.
* **port**: int: The port number for the route. Defaults to 80.
* **scheme**: int: The scheme used for traffic. Readonly.
* **status**: [RouteStatus](#routestatus): Status of a route.
* **url**: int: A stable URL that that can be used to route traffic to a resource. Readonly.

## HttpRouteGateway
### Properties
* **hostname**: string (Required): Specifies the public hostname for the route. Use '*' to listen on all hostnames.
* **rules**: [HttpRouteGatewayRules](#httproutegatewayrules): Dictionary of <HttpRouteGatewayRule>
* **source**: string: The gateway which this route is part of.

## HttpRouteGatewayRules
### Properties
### Additional Properties
* **Additional Properties Type**: [HttpRouteGatewayRule](#httproutegatewayrule)

## HttpRouteGatewayRule
### Properties
* **method**: string: Specifies the method to match on the incoming request.
* **path**: [HttpRouteGatewayPath](#httproutegatewaypath): Specifies path matching options to match requests on.

## HttpRouteGatewayPath
### Properties
* **type**: string: Specifies the path to match the incoming request.
* **value**: string: Specifies the type of matching to match the path on. Supported values: 'prefix', 'exact'.

## MicrosoftSQLDatabaseProperties
### Properties
* **database**: string: The name of the SQL database.
* **resource**: string: The ID of the SQL database to use for this resource.
* **server**: string: The fully qualified domain name of the SQL database.
* **status**: [ResourceStatusAutoGenerated](#resourcestatusautogenerated): Status of a resource.

## MongoDBResourceProperties
### Properties
* **host**: string: The host name of the MongoDB to which you are connecting
* **port**: int: The port value of the MongoDB to which you are connecting
* **resource**: string: The ID of the DB with Mongo API to use for this resource.
* **secrets**: [MongoDBResourcePropertiesSecrets](#mongodbresourcepropertiessecrets): Secrets provided by resources,
* **status**: [ResourceStatusAutoGenerated](#resourcestatusautogenerated): Status of a resource.

## MongoDBResourcePropertiesSecrets
### Properties
* **connectionString**: string: The connection string used to connect to this DB
* **password**: string: The password for this MongoDB instance
* **username**: string: The password for this MongoDB instance

## RabbitMQMessageQueueResourceProperties
### Properties
* **queue**: string (Required): The name of the queue
* **secrets**: [RabbitMQMessageQueueResourcePropertiesSecrets](#rabbitmqmessagequeueresourcepropertiessecrets): Secrets provided by resources,
* **status**: [ResourceStatusAutoGenerated](#resourcestatusautogenerated): Status of a resource.

## RabbitMQMessageQueueResourcePropertiesSecrets
### Properties
* **connectionString**: string: The connection string used to connect to this RabbitMQ instance

## RedisCacheResourceProperties
### Properties
* **host**: string: The host name of the redis cache to which you are connecting
* **port**: int: The port value of the redis cache to which you are connecting
* **resource**: string: The ID of the Redis cache to use for this resource
* **secrets**: [RedisCacheResourcePropertiesSecrets](#rediscacheresourcepropertiessecrets)
* **status**: [ResourceStatusAutoGenerated](#resourcestatusautogenerated): Status of a resource.

## RedisCacheResourcePropertiesSecrets
### Properties
* **connectionString**: string: The Redis connection string used to connect to the redis cache
* **password**: string: The password for this Redis instance

## VolumeProperties
* **Discriminator**: kind

### Base Properties
* **status**: [ResourceStatusAutoGenerated](#resourcestatusautogenerated): Status of a resource.
### AzureFileShareVolumeProperties
#### Properties
* **kind**: 'azure.com.fileshare' (Required): The VolumeProperties kind
* **resource**: string: The ID of the volume to use for this resource

### AzureKeyVaultVolumeProperties
#### Properties
* **certificates**: [AzureKeyVaultVolumePropertiesCertificates](#azurekeyvaultvolumepropertiescertificates): The KeyVault certificates that this volume exposes
* **keys**: [AzureKeyVaultVolumePropertiesKeys](#azurekeyvaultvolumepropertieskeys): The KeyVault keys that this volume exposes
* **kind**: 'azure.com.keyvault' (Required): The VolumeProperties kind
* **resource**: string: The ID of the keyvault to use for this volume resource
* **secrets**: [AzureKeyVaultVolumePropertiesSecrets](#azurekeyvaultvolumepropertiessecrets): The KeyVault secrets that this volume exposes


## AzureKeyVaultVolumePropertiesCertificates
### Properties
### Additional Properties
* **Additional Properties Type**: [CertificateObjectProperties](#certificateobjectproperties)

## CertificateObjectProperties
### Properties
* **alias**: string: File name when written to disk.
* **encoding**: 'base64' | 'hex' | 'utf-8': Encoding format. Default utf-8
* **format**: 'pem' | 'pfx': Certificate format. Default pem
* **name**: string (Required): The name of the certificate
* **value**: 'certificate' | 'privatekey' | 'publickey' (Required): Certificate object to be downloaded - the certificate itself, private key or public key of the certificate
* **version**: string: Certificate version

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

