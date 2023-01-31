# UCP Addressing Scheme

UCP uses its own addressing mechanism to be able to uniquely identify a resource. UCP provides a universal addressing scheme – the ability to define a stable identifier for resources across a variety of resource management systems. These identifiers are an extension and generalization of ARM’s resource ID concept. The goal with universal addressing is to define a single identifier format for describing the identity of any cloud resource on any control-plane system.

We define our universal-addressing concept as the UCP ID.

<br/><br/>

### Planes
UCP defines the concept of a plane to support ids that come from different types of systems (Azure vs GCP) or different instances of those systems (Azure Cloud vs Azure Gov Cloud). Planes have both a type and an instance.  

Plane types are well-known quantities to UCP. In general, the addition of a new plane type will require new code somewhere in the system unless the new plane type is ARM-like or UCP-native already.

Every resource resides in a particular plane, where a plane is the entity that is required to process that resource. For example:- 

* a Radius resource belongs to the “radius” plane and requires the Radius RP to deploy the resource. 

* an AWS resource belongs to the "aws" plane and requires AWS to deploy the resource.

 
Every resource will have a plane identifier followed by the resource ID. The plane type will determine what the resource ID is going to look like. For example, for an “azure” plane, the resourceID after the plane identifier will look like an ARM resource ID. Similarly, the resource ID for a resource in the "aws" plane will look like an AWS ID. Below are some examples clarifying this.


## Examples

| Plane |URL  |Notes  |
|--|--|--|
| Azure |/planes/azure/{azure cloud}/subscriptions/{sid}/resourceGroups/{rg}/providers/{provider name}/{resource type}/{resource name}  |  <azure cloud> could be Azure public cloud, Mooncake, Fairfax, etc.|
|AWS|/planes/aws /{partition-name}/service/{service-name}/region/{region}/account/{account-id}/providers/resourceType/{resource-id}||
|Radius|/planes/radius/local/resourceGroups/{rg}/providers/Applications.Core/containers/{name}|“local” refers to where Radius is running, in this case localhost, Note that here ”resourceGroups” is a Radius resource and not related to the ARM resourceGroups, Applications.Core refers to the Radius RP and container is an example of a Radius RP top level resource.|
|Kubernetes|/planes/Kubernetes/local/namespaces/{ns}/providers/apps/deployments/{name}||

We classify the planes as native and non-native. Note: Currently, we do not support Azure plane in UCP and instead Deployment Engine is configured to directly communicate with Azure. We plan on implementing this communication in UCP.

* Native Plane - A native plane is an entity which understands UCP addressing format and can directly process requests using the UCP addresses. An example is the Radius RP which is configured with routes for /planes/radius/local/...

* Non-Native Plane - A non-native plane is for which UCP needs to perform address translation from the UCP addresses to the plane specific addresses. An example is the Azure or AWS plane.

<br/><br/>

### Resource Group
A resource group is used to organize user resources. Note that even though conceptually this is similar to an Azure resource group, it is not the same and is a UCP resource independent of Azure.
