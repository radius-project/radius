# Applications.Messaging @ 2023-10-01-preview

## Resource Applications.Messaging/rabbitMQQueues@2023-10-01-preview
* **Valid Scope(s)**: Unknown
### Properties
* **apiVersion**: '2023-10-01-preview' (ReadOnly, DeployTimeConstant): The resource api version
* **id**: string (ReadOnly, DeployTimeConstant): The resource id
* **location**: string (Required): The geo-location where the resource lives
* **name**: string (Required, DeployTimeConstant): The resource name
* **properties**: [RabbitMQQueueProperties](#rabbitmqqueueproperties): RabbitMQQueue portable resource properties
* **systemData**: [SystemData](#systemdata) (read-only): Metadata pertaining to creation and last modification of the resource.
* **tags**: [TrackedResourceTags](#trackedresourcetags): Resource tags.
* **type**: 'Applications.Messaging/rabbitMQQueues' (read-only, deploy-time constant): The resource type

## Function listSecrets (Applications.Messaging/rabbitMQQueues@2023-10-01-preview)
* **Resource**: Applications.Messaging/rabbitMQQueues
* **ApiVersion**: 2023-10-01-preview
* **Input**: any
* **Output**: [RabbitMQListSecretsResult](#rabbitmqlistsecretsresult)

## RabbitMQQueueProperties
### Properties
* **application**: string: Fully qualified resource ID for the application that the portable resource is consumed by (if applicable)
* **environment**: string (required): Fully qualified resource ID for the environment that the portable resource is linked to
* **host**: string: The hostname of the RabbitMQ instance
* **port**: int: The port of the RabbitMQ instance. Defaults to 5672
* **provisioningState**: 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' (read-only): Provisioning state of the portable resource at the time the operation was called
* **queue**: string: The name of the queue
* **recipe**: [Recipe](#recipe): The recipe used to automatically deploy underlying infrastructure for a portable resource
* **resourceProvisioning**: 'manual' | 'recipe': Specifies how the underlying service/resource is provisioned and managed. Available values are 'recipe', where Radius manages the lifecycle of the resource through a Recipe, and 'manual', where a user manages the resource and provides the values.
* **resources**: [ResourceReference](#resourcereference)[]: List of the resource IDs that support the rabbitMQ resource
* **secrets**: [RabbitMQSecrets](#rabbitmqsecrets): The connection secrets properties to the RabbitMQ instance
* **status**: [ResourceStatus](#resourcestatus) (read-only): Status of a resource.
* **tls**: bool: Specifies whether to use SSL when connecting to the RabbitMQ instance
* **username**: string: The username to use when connecting to the RabbitMQ instance
* **vHost**: string: The RabbitMQ virtual host (vHost) the client will connect to. Defaults to no vHost.

## Recipe
### Properties
* **name**: string (required): The name of the recipe within the environment to use
* **parameters**: any: Any object

## ResourceReference
### Properties
* **id**: string (required): Resource id of an existing resource

## RabbitMQSecrets
### Properties
* **password**: string: The password used to connect to the RabbitMQ instance
* **uri**: string: The connection URI of the RabbitMQ instance. Generated automatically from host, port, SSL, username, password, and vhost. Can be overridden with a custom value

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
* **kind**: 'kubernetes' (required): Discriminator property for EnvironmentCompute.
* **namespace**: string (required): The namespace to use for the environment.


## IdentitySettings
### Properties
* **kind**: 'azure.com.workload' | 'undefined' (required): IdentitySettingKind is the kind of supported external identity setting
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

## RabbitMQListSecretsResult
### Properties
* **password**: string (read-only): The password used to connect to the RabbitMQ instance
* **uri**: string (read-only): The connection URI of the RabbitMQ instance. Generated automatically from host, port, SSL, username, password, and vhost. Can be overridden with a custom value

