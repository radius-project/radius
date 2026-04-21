# Title

* **Author**: Nithya Subramanian (@nithyatsu)

## Overview

Radius offers an application resource which teams can use to define and deploy their entire application, including all of the compute, relationships, and infrastructure that make up the application. Within an application deployed with Radius, developers can express both the resources (containers, databases, message queues, etc.), as well as all the relationships between them. This forms the Radius application graph. This graph allows Radius to understand the relationships between resources, simplifying the deployment and configuration of the application. Plus, it allows the users to visualize their application in a way that is more intuitive than a list of resources.

Today, Radius supports `rad app connections` CLI command to retrieve this graph. However, much of our graph building logic resides at the Radius client. We should build an API which can return the serialized graph so that any Radius client can easily retrieve its Application as a graph and utilize it.

## Terms and definitions


| Term | Definition |
|---|---|
| Application Graph | Directed graph representing an Application as all of its resources and the relationship between these resources|
| rootScope | the current UCP Scope |

## Objectives

> **Issue Reference:** 
https://github.com/radius-project/radius/issues/6337

### Goals

* Radius should provide an API which can retrieve the Application Graph for a given Application ID.

### Non goals

We would want to address below goals in future:
* Performance of the API should be improved. 
* We should add metadata that could make the Application graph interactive.

### User scenarios (optional)

#### User story 1

As a new Developer, I would like to get a High Level Overview of the Application I am working on, showing all the resources that make up my application and the relationships between them.


## Design

Since we are querying an Application's details, UCP should proxy this API to Applications.Core resource provider. We should be able to reuse much of the ApplicationGraph structure we currently have in  `cli` for supporting the `rad app connections` command. The ApplicationGraph is a list of Resources, with each Resource including information about its Dependencies(Connections). 

The Applications.Core RP should be able to 
1. query all resources in the `rootScope`
2. filter resources relevant to the given Application based on the app.id field
3. construct the graph object based on `connections`.

We should be able to handle connections that take a resourceID for destination as well as those which take a URL. 

As requirement evolves, we would be able to add properties such as a repository link to a container or a health url and retrieve these as part of application graph. This graph object could then be consumed by react components to provide the desired UX experience. 


### API design

The API to retrieve an Application Graph looks like

`POST /{rootScope}/providers/Applications.Core/applications/{applicationName}/getGraph`

  - Description: retrieve {applicationName}'s  Application Graph.
  - Type: ARM Synchronous

Where

`/{rootScope}/providers/Applications.Core/applications/{applicationName}` is the resource ID of the Application for which we want the graph.

`getGraph` is the custom action on this resource. Ref. [ARM Custom Actions](https://learn.microsoft.com/en-us/azure/azure-resource-manager/custom-providers/custom-providers-action-endpoint-how-to)



Possible Responses

* `HTTP 200 OK` with Serialized `ApplicationGraph` as response data.
* `HTTP 404 Not Found` for Application Not Found

***Model changes***

Addition of ApplicationGraphResponse type and getGraph method to applications.tsp

```
@doc("Describes the application architecture and its dependencies.")
model ApplicationGraphResponse {
  @doc("The resources in the application graph.")
  @extension("x-ms-identifiers", ["id"])
  resources: Array<ApplicationGraphResource>;
}

@doc("Describes the connection between two resources.")
model ApplicationGraphConnection {
  @doc("The resource ID ")
  id: string;

  @doc("The direction of the connection. 'Outbound' indicates this connection specifies the ID of the destination and 'Inbound' indicates indicates this connection specifies the ID of the source.")
  direction: Direction;
}

@doc("The direction of a connection.")
enum Direction {
  @doc("The resource defining this connection makes an outbound connection resource specified by this id.")
  Outbound,

  @doc("The resource defining this connection accepts inbound connections from the resource specified by this id.")
  Inbound,
}

@doc("Describes a resource in the application graph.")
model ApplicationGraphResource {
  @doc("The resource ID.")
  id: string;

  @doc("The resource type.")
  type: string;

  @doc("The resource name.")
  name: string;

  @doc("The resources that comprise this resource.")
  @extension("x-ms-identifiers", ["id"])
  resources: Array<ApplicationGraphOutputResource>;

  @doc("The connections between resources in the application graph.")
  @extension("x-ms-identifiers",[])
  connections: Array<ApplicationGraphConnection>;

  @doc("provisioningState of this resource") 
  provisioningState?: string
}

@doc("Describes an output resource that comprises an application graph resource.")
model ApplicationGraphOutputResource {
  @doc("The resource ID.")
  id: string;

  @doc("The resource type.")
  type: string;

  @doc("The resource name.")
  name: string;
}
```

```
 @doc("Gets the application graph and resources.")
  @action("getGraph")
  getGraph is ArmResourceActionSync<
    ApplicationResource,
    {},
    ApplicationGraphResponse,
    UCPBaseParameters<ApplicationResource>
  >;
```

***Example***

`rad deploy app.bicep`

Contents of `app.bicep`

```
import radius as radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-gateway'
  location: location
  properties: {
    environment: environment
  }
}

resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'http-gtwy-gtwy'
  location: location
  properties: {
    application: app.id
    routes: [
      {
        path: '/'
        destination: frontendRoute.id
      }
      {
        path: '/backend1'
        destination: backendRoute.id
      }
      {
        // Route /backend2 requests to the backend, and
        // transform the request to /
        path: '/backend2'
        destination: backendRoute.id
        replacePrefix: '/'
      }
    ]
  }
}

resource frontendRoute 'Applications.Core/httpRoutes@2023-10-01-preview' = {
  name: 'http-gtwy-front-rte'
  location: location
  properties: {
    application: app.id
    port: 81
  }
}

resource frontendContainer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'http-gtwy-front-ctnr'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
          provides: frontendRoute.id
        }
      }
      readinessProbe: {
        kind: 'httpGet'
        containerPort: port
        path: '/healthz'
      }
    }
    connections: {
      backend: {
        source: backendRoute.id
      }
    }
  }
}

resource backendRoute 'Applications.Core/httpRoutes@2023-10-01-preview' = {
  name: 'http-gtwy-back-rte'
  location: location
  properties: {
    application: app.id
  }
}

resource backendContainer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'http-gtwy-back-ctnr'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      env: {
        gatewayUrl: gateway.properties.url
      }
      ports: {
        web: {
          containerPort: port
          provides: backendRoute.id
        }
      }
      readinessProbe: {
        kind: 'httpGet'
        containerPort: port
        path: '/healthz'
      }
    }
  }
}

```

Assuming we set up Radius with the default  `rad init` command, Rest API For querying the above Application's graph would look like

`POST /ucphostname:ucpport/apis/api.ucp.dev/v1alpha3/planes/radius/local/resourceGroups/default/providers/Applications.Core/applications/corerp-resources-gateway/getGraph?api-version=2023-10-01-preview`

Response indicating Success would be 

`HTTP 200 OK` With response body as below. 

```
{
    "resources": [
        {
            "connections": [
                {
                    "direction": "Inbound",
                    "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/http-gtwy-front-ctnr"
                }
            ],
            "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/httpRoutes/http-gtwy-front-rte",
            "name": "http-gtwy-front-rte",
            "resources": [
                {
                    "id": "/planes/kubernetes/local/namespaces/default-corerp-resources-gateway/providers/core/Service/http-gtwy-front-rte",
                    "name": "http-gtwy-front-rte",
                    "type": "core/Service"
                }
            ],
            "type": "Applications.Core/httpRoutes",
            "provisioningState": "Succeeded"
        },
        {
            "connections": [
                {
                    "direction": "Outbound",
                    "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/httpRoutes/http-gtwy-back-rte"
                }
            ],
            "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/http-gtwy-back-ctnr",
            "name": "http-gtwy-back-ctnr",
            "resources": [
                {
                    "id": "/planes/kubernetes/local/namespaces/default-corerp-resources-gateway/providers/apps/Deployment/http-gtwy-back-ctnr",
                    "name": "http-gtwy-back-ctnr",
                    "type": "apps/Deployment"
                },
                {
                    "id": "/planes/kubernetes/local/namespaces/default-corerp-resources-gateway/providers/core/ServiceAccount/http-gtwy-back-ctnr",
                    "name": "http-gtwy-back-ctnr",
                    "type": "core/ServiceAccount"
                },
                {
                    "id": "/planes/kubernetes/local/namespaces/default-corerp-resources-gateway/providers/rbac.authorization.k8s.io/Role/http-gtwy-back-ctnr",
                    "name": "http-gtwy-back-ctnr",
                    "type": "rbac.authorization.k8s.io/Role"
                },
                {
                    "id": "/planes/kubernetes/local/namespaces/default-corerp-resources-gateway/providers/rbac.authorization.k8s.io/RoleBinding/http-gtwy-back-ctnr",
                    "name": "http-gtwy-back-ctnr",
                    "type": "rbac.authorization.k8s.io/RoleBinding"
                }
            ],
            "type": "Applications.Core/containers",
            "provisioningState": "Succeeded"
        },
        {
            "connections": [
                {
                    "direction": "Inbound",
                    "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/httpRoutes/http-gtwy-back-rte"
                },
                {
                    "direction": "Inbound",
                    "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/httpRoutes/http-gtwy-back-rte"
                },
                {
                    "direction": "Outbound",
                    "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/httpRoutes/http-gtwy-front-rte"
                }
            ],
            "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/http-gtwy-front-ctnr",
            "name": "http-gtwy-front-ctnr",
            "resources": [
                {
                    "id": "/planes/kubernetes/local/namespaces/default-corerp-resources-gateway/providers/apps/Deployment/http-gtwy-front-ctnr",
                    "name": "http-gtwy-front-ctnr",
                    "type": "apps/Deployment"
                },
                {
                    "id": "/planes/kubernetes/local/namespaces/default-corerp-resources-gateway/providers/core/Secret/http-gtwy-front-ctnr",
                    "name": "http-gtwy-front-ctnr",
                    "type": "core/Secret"
                },
                {
                    "id": "/planes/kubernetes/local/namespaces/default-corerp-resources-gateway/providers/core/ServiceAccount/http-gtwy-front-ctnr",
                    "name": "http-gtwy-front-ctnr",
                    "type": "core/ServiceAccount"
                },
                {
                    "id": "/planes/kubernetes/local/namespaces/default-corerp-resources-gateway/providers/rbac.authorization.k8s.io/Role/http-gtwy-front-ctnr",
                    "name": "http-gtwy-front-ctnr",
                    "type": "rbac.authorization.k8s.io/Role"
                },
                {
                    "id": "/planes/kubernetes/local/namespaces/default-corerp-resources-gateway/providers/rbac.authorization.k8s.io/RoleBinding/http-gtwy-front-ctnr",
                    "name": "http-gtwy-front-ctnr",
                    "type": "rbac.authorization.k8s.io/RoleBinding"
                }
            ],
            "type": "Applications.Core/containers",
            "provisioningState": "Succeeded"
        },
        {
            "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/gateways/http-gtwy-gtwy",
            "name": "http-gtwy-gtwy",
            "resources": [
                {
                    "id": "/planes/kubernetes/local/namespaces/default-corerp-resources-gateway/providers/projectcontour.io/HTTPProxy/http-gtwy-back-rte",
                    "name": "http-gtwy-back-rte",
                    "type": "projectcontour.io/HTTPProxy"
                },
                {
                    "id": "/planes/kubernetes/local/namespaces/default-corerp-resources-gateway/providers/projectcontour.io/HTTPProxy/http-gtwy-front-rte",
                    "name": "http-gtwy-front-rte",
                    "type": "projectcontour.io/HTTPProxy"
                },
                {
                    "id": "/planes/kubernetes/local/namespaces/default-corerp-resources-gateway/providers/projectcontour.io/HTTPProxy/http-gtwy-gtwy",
                    "name": "http-gtwy-gtwy",
                    "type": "projectcontour.io/HTTPProxy"
                }
            ],
            "type": "Applications.Core/gateways",
            "provisioningState": "Succeeded"
        },
        {
            "connections": [
                {
                    "direction": "Inbound",
                    "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/http-gtwy-back-ctnr"
                }
            ],
            "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/httpRoutes/http-gtwy-back-rte",
            "name": "http-gtwy-back-rte",
            "resources": [
                {
                    "id": "/planes/kubernetes/local/namespaces/default-corerp-resources-gateway/providers/core/Service/http-gtwy-back-rte",
                    "name": "http-gtwy-back-rte",
                    "type": "core/Service"
                }
            ],
            "type": "Applications.Core/httpRoutes",
            "provisioningState": "Succeeded"
        }
    ]
}
```


## Alternatives considered

There are multiple ways of representing a graph. We considered below model as an option since it is optimized for bandwidth. However, the model does not work well for pagination (to be added in future), which would be a requirement to support large applications.  


```
@doc("Describes the application architecture and its dependencies.")
model ApplicationGraphResponse {
  @doc("The connections between resources in the application graph.")
  @extension("x-ms-identifiers",[])
  connections: Array<ApplicationGraphConnection>;

  @doc("The resources in the application graph.")
  @extension("x-ms-identifiers", ["id"])
  resources: Array<ApplicationGraphResource>;
}

@doc("Describes the connection between two resources.")
model ApplicationGraphConnection {
  @doc("The source of the connection.")
  source: string;

  @doc("The destination of the connection.")
  destination: string;
}

@doc("Describes a resource in the application graph.")
model ApplicationGraphResource {
  @doc("The resource ID.")
  id: string;

  @doc("The resource type.")
  type: string;

  @doc("The resource name.")
  name: string;

  @doc("The resources that comprise this resource.")
  @extension("x-ms-identifiers", ["id"])
  resources: Array<ApplicationGraphResource>;
}

@doc("Describes an output resource that comprises an application graph resource.")
model ApplicationGraphOutputResource {
  @doc("The resource ID.")
  id: string;

  @doc("The resource type.")
  type: string;

  @doc("The resource name.")
  name: string;
}
```


## Test plan

We should add a E2E that deploys an application and tests if the ApplicationGraph object that can be retrieved using the new API as expected. 

We should also add UTs as needed for all the functions introduced/changed.


## Monitoring

Trace and Metrics will be generated automatically for the new API. 
We should return appropriate errors so that logs are generated for these conditions.

## Development plan

1. Support new Route in Applications.Core 
2. Implement controller and UT for the new Route 
3. Implement E2E that tests the new API
4. Add documentation
5. Rewrite rad app connections using the new API, update relevant tests.


## Open issues

1. The serialized ApplicationGraph in HTTP response could be quite heavy for huge Applications, consuming more network bandwidth. We might have to make this better based on the requirement. 

2. We currently do not have a efficient query to retrieve only the resources in a given application. This might need to be revisited.
