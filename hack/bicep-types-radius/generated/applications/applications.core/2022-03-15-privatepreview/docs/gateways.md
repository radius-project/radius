## Resource Applications.Core/gateways@2022-03-15-privatepreview

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **apiVersion** | '2022-03-15-privatepreview' |  ReadOnly, DeployTimeConstant | The resource api version |
| **id** | string |  ReadOnly, DeployTimeConstant | The resource id |
| **location** | string |  Required | The geo-location where the resource lives |
| **name** | string |  Required, DeployTimeConstant | The resource name |
| **properties** | [GatewayProperties](#gatewayproperties) |  | Gateway properties |
| **systemData** | [SystemData](#systemdata) |  ReadOnly | Metadata pertaining to creation and last modification of the resource. |
| **tags** | [TrackedResourceTags](#trackedresourcetags) |  | Resource tags. |
| **type** | 'Applications.Core/gateways' |  ReadOnly, DeployTimeConstant | The resource type |

## GatewayProperties

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **application** | string |  Required | Fully qualified resource ID for the application that the portable resource is consumed by |
| **environment** | string |  | Fully qualified resource ID for the environment that the portable resource is linked to (if applicable) |
| **hostname** | [GatewayHostname](#gatewayhostname) |  | Declare hostname information for the Gateway. Leaving the hostname empty auto-assigns one: mygateway.myapp.PUBLICHOSTNAMEORIP.nip.io. |
| **internal** | bool |  | Sets Gateway to not be exposed externally (no public IP address associated). Defaults to false (exposed to internet). |
| **provisioningState** | 'Accepted' | 'Canceled' | 'Deleting' | 'Failed' | 'Provisioning' | 'Succeeded' | 'Updating' |  ReadOnly | Provisioning state of the portable resource at the time the operation was called |
| **routes** | [GatewayRoute](#gatewayroute)[] |  Required | Routes attached to this Gateway |
| **status** | [ResourceStatus](#resourcestatus) |  ReadOnly | Status of a resource. |
| **tls** | [GatewayTls](#gatewaytls) |  | TLS configuration definition for Gateway resource. |
| **url** | string |  ReadOnly | URL of the gateway resource. Readonly |

## GatewayHostname

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **fullyQualifiedHostname** | string |  | Specify a fully-qualified domain name: myapp.mydomain.com. Mutually exclusive with 'prefix' and will take priority if both are defined. |
| **prefix** | string |  | Specify a prefix for the hostname: myhostname.myapp.PUBLICHOSTNAMEORIP.nip.io. Mutually exclusive with 'fullyQualifiedHostname' and will be overridden if both are defined. |

## GatewayRoute

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **destination** | string |  | The HttpRoute to route to. Ex - myserviceroute.id. |
| **path** | string |  | The path to match the incoming request path on. Ex - /myservice. |
| **replacePrefix** | string |  | Optionally update the prefix when sending the request to the service. Ex - replacePrefix: '/' and path: '/myservice' will transform '/myservice/myroute' to '/myroute' |

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


## IdentitySettings

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **kind** | 'azure.com.workload' | 'undefined' |  Required | IdentitySettingKind is the kind of supported external identity setting |
| **oidcIssuer** | string |  | The URI for your compute platform's OIDC issuer |
| **resource** | string |  | The resource ID of the provisioned identity |

## OutputResource

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **id** | string |  | The UCP resource ID of the underlying resource. |
| **localId** | string |  | The logical identifier scoped to the owning Radius resource. This is only needed or used when a resource has a dependency relationship. LocalIDs do not have any particular format or meaning beyond being compared to determine dependency relationships. |
| **radiusManaged** | bool |  | Determines whether Radius manages the lifecycle of the underlying resource. |

## GatewayTls

### Properties

| Property | Type | Flags | Description |
|----------|------|-------|-------------|
| **certificateFrom** | string |  | The resource id for the secret containing the TLS certificate and key for the gateway. |
| **minimumProtocolVersion** | '1.2' | '1.3' |  | Tls Minimum versions for Gateway resource. |
| **sslPassthrough** | bool |  | If true, gateway lets the https traffic sslPassthrough to the backend servers for decryption. |

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

