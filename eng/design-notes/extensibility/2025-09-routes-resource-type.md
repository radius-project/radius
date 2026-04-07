# Routes Resource Type Definition

* **Author**: Zach Casper (@zachcasper)

## Overview

The compute extensibility project is implementing Recipe-backed Resource Types for the core Radius Resource Types including Containers, Volumes, Secrets, and Gateways. As part of this effort, Recipes are being developed replacing the imperative code in Applications RP. Because of this, we are taking the opportunity to re-examine the schema and make adjustments as needed. 

This document proposes a new resource type, Routes, which replaces the existing Gateways resource. The rationale is explained within.

## Objectives

### Goals

* Increase the separation of concerns between platform engineers and developers. The existing Gateways resource type blurs this line.
* Enable developers to use the Routes Resource Type for the vast majority of their use cases without platform engineers needing to modify the Resource Type definition. 
* Ensure that Routes is modeled such that other container platforms can be implemented via Recipes in the future. This includes AWS Elastic Container Service (ECS), Azure Container Apps (ACA), Azure Container Instances (ACI), and Google Cloud Run. 
* Ensure the implementation of Routes is contained within the Recipe only. There should be no changes required of Radius itself.

### Non goals

This document is focused solely on the Routes Resource Type. Volumes, Secrets, and Containers are discussed elsewhere.

## Challenges with `Applications.Core/gateways`

The current version of Gateways has several challenges.

### Dependency on Contour

Today, Radius takes a very opinionated approach to the implementation of L7 ingress and requires Contour to be installed as part of a Radius installation. Unfortunately, Contour is far from universally used. Kubernetes platform engineers have a wide variety of Gateway Controllers to select from. Furthermore, Contour is not the leading option with NGINX being the most popular and Cilium becoming more popular given its use of eBPF.

### Modeled using HTTPProxy rather than the Gateway API

The Containers Resource Type is modeled around the Contour-specific HTTPProxy resource. However, the Kubernetes ecosystem is aligning on the Gateway API which was made generally available in November 2023. 

### Not integrated with Containers

The Gateways Resource Type has a `destination` property to specify where HTTP requests should be routed to. However, this value is deployment platform-specific. The documentation has an example value of `http://backend:80`. While it may seem as though this is the Container resource named backend with a container port of 80, it is actually the Kubernetes service name backend. This breaks the container platform abstraction and requires developers to have platform-specific knowledge.

### Developers are required to configure TLS

The Gateways Resource Type has several properties for configuring TLS including `sslPassthrough`, `hostname`, `certificateFrom`, and `minimumProtocolVersion`. These are all platform-specific properties that the platform engineer should be configuring, not the developer.

## Container Platform Ingress Patterns

The user experience for configuring ingress on Kubernetes, ECS, ACA, ACI, and Cloud Run was examined. Kubernetes and ECS are very similar—each has a load balancer, a listener, and a set of routes with matching rules. ACA and Cloud Run take a different approach with built-in ingress capabilities. Containers are deployed as services and ingress is simply enabled. Routing rules are possible with ACA and Cloud Run but both these services are more inclined to provide a fully-qualified domain name for each service reducing the need for routing rules.

| Platform                  | Load Balancer                                                | Listener                                                     | Route                                                        |
| ------------------------- | ------------------------------------------------------------ | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Kubernetes                | [Gateway](https://gateway-api.sigs.k8s.io/reference/spec/#gateway) | [Listener](https://gateway-api.sigs.k8s.io/reference/spec/#listener) | [Routes](https://gateway-api.sigs.k8s.io/reference/spec/#httproute) |
| AWS ECS                   | Application Load Balancer                                    | ALB Listener                                                 | [Listener Rule](https://docs.aws.amazon.com/elasticloadbalancing/latest/APIReference/API_CreateRule.html) |
| Azure Container Apps      | Built-in                                                     | Built-in                                                     | [httpRouteConfig](https://learn.microsoft.com/en-us/azure/templates/microsoft.app/managedenvironments/httprouteconfigs?pivots=deployment-language-bicep) |
| Azure Container Instances | Not available                                                | Not available                                                | Not available                                                |
| Google Cloud Run          | Built-in                                                     | Built-in                                                     | [URL Maps](https://cloud.google.com/load-balancing/docs/url-map) when using Application Load Balancer |

## Proposed new `Radius.Compute/routes`

It is proposed that the Gateways Resource Type be replaced by a Routes Resource Type. The Routes Resource Type allows developers to describe network routes to their application's services. 

Routes are HTTP, TLS, TCP, and UDP network routes. They are not gateways, ingress controllers, or load balancers. According the Kubernetes documentation, gateways "represents an instance of a service-traffic handling ***infrastructure*** by binding Listeners to a set of IP addresses." In other words, gateways are the infrastructure needed to implement the routes specified by the developer.

Routes align with the Kubernetes Gateway API. The Gateway API is separated into separate API resources which map to platform engineers and developers respectively:

* `Gateway` (this is the Kubernetes Gateway API):  
  * `GatewayClass`: Defines the specific controller used 
  * `Listeners`: Define the hostnames, ports, protocol, termination, TLS settings and which routes can be attached to a listener.
  * `Addresses`: The network addresses for this Gateway
  
* Routes: Defines a set of rules to match and service destinations (`backendRefs`). 

  * `HTTPRoute`: L7 ingress with support for matching based on the hostname and HTTP header
  * `TCPRoute`: L4 ingress with no support for matching (all traffic to the requested port is forwarded to the backendRef)
  * `TLSRoute`: L4 ingress only with the ability to match based on Server Name Indication (SNI) which is equivalent to hostname in TLS

  * `UDPRoute`: Same as TCPRoutes

Routes also align with routes in ACA, Cloud Run, and ECS.

> [!NOTE]
>
> The L4 ingress enabled by TCPRoute, TLSRoute, and UDPRoute are distinct from the basic L4 networking capability of Kubernetes Pods and Services. When a containerPort is specified on a container, Radius deploys a Kubernetes Service of type `ClusterIP` which enables intra-cluster service-to-service connectivity. Routes enable ingress from outside of the cluster. 

## Proposed schema

The Routes Resource Type definition is modeled primarily on the Kubernetes [HTTPRoute](https://gateway-api.sigs.k8s.io/reference/spec/#httproute) and the [AWS ALB Rule](https://docs.aws.amazon.com/elasticloadbalancing/latest/APIReference/API_RuleCondition.html). It has these high-level schema properties:

`kind`: Routes will be a single Resource Type with a `kind` enum which includes: HTTP, TCP, TLS, and UDP. gRPC is a potential future enhancement.

`rules[]`:

- `matches[]`:
  - `path`: The HTTP request path to match. Trailing space is ignored. Requests for `/abc`, `/abc/`, and ``/abc/def/` will all match `/abc`.
  - `httpHeaders[]`: HTTP headers to match. Specify only when kind is HTTP.
    - `name`: The HTTP header name to match
    - `value`: The value of the HTTP header to match
  - `httpMethod`: The HTTP method to match. Specify only when kind is HTTP. Enum: [GET HEAD POST PUT DELETE CONNECT OPTIONS TRACE PATCH]
  - `queryParams[]`: HTTP query parameters to match. Specify only when kind is HTTP.
    - `name`: The query parameter name to match
    - `value`: The value of the query parameter to match

- `destinationContainerId`: The Radius Container resource ID to route requests to.

## Developer Experience

### User Story 1: Service-to-service connectivity

***As a developer, my application has multiple micro-services which need to communicate with each other.***

> [!NOTE]
>
> This user story is not related to Routes. It is included to demonstrate the multitude of networking use cases.

Each of the developer's services opens a socket on a port then specifies that port in the `containerPort` property.

```yaml
resource myApp 'Applications.Core/applications@2023-10-01-preview' = { ... }

resource svcA 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'svcA'
  properties: {
    application: myApp.id
    container: {
      image: 'svcA:latest'
      ports: {
        web: {
          containerPort: 8080
        }
      }
    }
  }
}

resource svcB 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'svcB'
  properties: {
    application: myApp.id
    container: {
      image: 'svcB:latest'
      ports: {
        web: {
          containerPort: 8080
        }
      }
    }
  }
}
```

When deploying the above example, Radius creates on the Kubernetes cluster:

* Deployment for svcA
* Deployment for svcB
* Service for svcA of type ClusterIP
* Service for svcB of type ClusterIP

svcA can communicate with svcB using built-in Kubernetes service discovery (there is a `svcA.<namespace>.cluster.local` DNS entry and the kubeproxy handles the IP routing. 

If a containerPort had not been specified, the two Services would not have been created.

### User Story 2: Responding to external traffic

***As a developer, I need my application to accept connections from external clients.***

Prior to developers using Routes, the platform engineer must: 

1. Configure a Gateway Controller 
2. Set the `hostnames` and `parentRef` Recipe parameters on the Routes Recipe (see Recipe Behavior below)

The developer then defines the application.

```yaml
resource myApp 'Applications.Core/applications@2023-10-01-preview' = { ... }

resource frontend 'Radius.Compute/containers@2025-08-01-preview' = { ... }
resource accounts 'Radius.Compute/containers@2025-08-01-preview' = { ... }

resource ingressRule 'Radius.Compute/routes@2025-08-01-preview' = {
  kind: 'HTTP'
  rules: [
    {
      matches: {
        path: '/'
      }
      destinationContainerId: frontend.id
    }
    {
      matches: {
        path: '/accounts'
      }
      destinationContainerId: accounts.id
    }
  ]
```

When deployed, an HTTPRoute resource is deploy to the Kubernetes cluster. This instructs the Gateway Controller to route HTTP requests for `/` to the frontend service and requests for `/accounts` to the accounts service.

## Recipe Behavior

### Kubernetes

The Routes Recipe will deploy a HTTPRoute, TCPRoute, TLSRoute, or UDPRoute based on the kind. The Platform engineer is expected to:

1. Deploy and configure their ingress controller of choice
2. Create a Kubernetes Gateway resource 
3. Set Routes Recipe parameters in the Recipe Pack or on the Environment

The Routes Recipe will have two parameters:

* `hostnames`: The HTTP Host header to match. If not specified, the `hostnames` property on the HTTPRoute resource is omitted. This will result in all requests sent to the Listener that match the Route rules being sent to the destination Container.
* `parentRef`: The name and namespace of the already deployed Kubernetes Gateway resource

The following features are not implemented:

* [`HTTPRoute.spec.rules.filters`](https://gateway-api.sigs.k8s.io/reference/spec/#httproutefilter): ECS does not support filters so this is excluded.

Other notes:

* The `HTTPRoute.spec.rules.match.path.type` should be `PathPrefix`
* The `HTTPRoute.spec.rules.match.headers.type` should be `Exact`
* The `HTTPRoute.spec.rules.match.queryParams.type` should be `Exact`

## Current Gateway Functionality Omitted

### Hostname generation

Because Gateways is tightly coupled with Contour, the hostname behavior is very specific. Contour and Gateways creates a hostname in the `nip.io` domain unless specified by the developer. Nip.io is a third-party DNS service useful for prototyping. Because Routes is controller-agnostic, the current hostname behavior is removed and delegated to the platform engineer and their Gateway Controller of choice.

### TLS configuration

Gateways expects the developer to provide a certificate stored in a Radius Secret. Because TLS is Gateway Controller-specific, TLS configuration is delegated to the platform engineer and their Gateway Controller of choice. 

### Miscellaneous Gateway routes properties

The following Gateways properties no longer supported since they are Gateway Controller-specific:

* `routes.replacePrefix`
* `routes.enableWebsockets`
* `routes.timeoutPolicy.request`
* `routes.timeoutPolicy.backendRequest`

