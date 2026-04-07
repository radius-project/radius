# Radius Dashboard Component Threat Model

- **Author**: @nithyatsu

## Overview

This document provides a threat model for the Radius Dashboard component. It identifies potential security threats to this part of Radius and suggests possible mitigation. The document includes an analysis of the system, its assets, identified threats, and recommended security measures to protect the system.

The Radius Dashboard component provides the frontend experience for Radius. 
It provides visual and textual representation of user's applications, environments and recipes.

## Terms and Definitions

| Term                  | Definition      |
| --------------------- | ----------------------------- |
| UCP | Universal Control Plane for Radius |
| DoS | Denial of Service |
| React Application | A web application built using the React library, which allows for the creation of dynamic and interactive user interfaces through a component-based architecture|

## System Description

The Dashboard component is an instance of [Backstage](https://backstage.io/). We customize Backstage by installing a Radius plugin and the community-supported Kubernetes plugin. The Dashboard is a client of the Radius API. It queries the graph of an application or a list of environment and constructs a visual representation of the response.

### Architecture

![Dashboard Architecture](2024-08-dashboard-component-threat-model/dashboard-arch.png)

Since we are shipping an instance of Backstage, it is essential to first examine the Backstage architecture. Backstage provides a core Single Page Application (SPA), a core backend, and the ability to configure a desired database. The core functionality can be enhanced using plugins.

We have implemented a Radius SPA plugin that integrates with the core Backstage SPA, and a Radius backend plugin that integrates with the core Backstage backend. While the backend can technically be accessed directly, our setup restricts this access. We configured the Radius Dashboard App to use a lightweight, in-memory SQLite database, which stores no data and is not accessible directly from outside.

This application is run as dashboard pod in radius-system namespace along with other Radius pods. 

### Implementation Details

The Radius Dashboard is an instance of Backstage, making it dependent on the Backstage framework for both display and backend functionality. For detailed information on Backstage's threat model, refer to the [Backstage Threat Model](https://backstage.io/docs/overview/threat-model/).

The application uses two config files: app-config.yaml, 
app-config.dashboard.yaml. 

app-config.dashboard.yaml has settings for dashboard installation as part of Radius installation. This merges and overlays the configs in app-config.yaml. 

We have used these files to configure our application to

1. allow communication using Radius API. The data for rendering Radius visuals is obtained by calling different Radius APIs. 

```
app-config.dashboard.yaml

kubernetes:
  serviceLocatorMethod:
    type: singleTenant
  # Use the local proxy on localhost:8001 to talk to the local Kubernetes cluster.
  clusterLocatorMethods:
    - type: config
      clusters:
        - name: self
          # The URL to the in-cluster Kubernetes API server.
          # Backstage docs state it should be ignored when in-cluster, but it appears to be used.
          url: https://kubernetes.default.svc.cluster.local
```

2. not expose backend directly.

```
app-config.yaml

backend:
  # Note that the baseUrl should be the URL that the browser and other clients
  # should use when communicating with the backend, i.e. it needs to be
  # reachable not just from within the backend host, but from all of your
  # callers. When its value is "http://localhost:7007", it's strictly private
  # and can't be reached by others.
  baseUrl: http://localhost:7007
  # The listener can also be expressed as a single <host>:<port> string. In this case we bind to
  # all interfaces, the most permissive setting. The right value depends on your specific deployment.
  listen: ':7007'
```

3.  use sqlite DB. This is light weight, not accessible to users directly and contains no useful information.
  
```
app-config.yaml :

backend:
  database:
    client: better-sqlite3
    connection: ':memory:'
```

At present, Dashboard can only present the Radius application metadata visually. It has no ability to Create, Modify, Update or Delete any of the Radius application resources. This eliminates the scope of threats that require "write" action using the Dashboard portal.

#### Storage of secrets

None

#### Data Serialization / Formats

None

### Cryptography 

None

### Clients

The primary user of Dashboard is browser. At present, we don't have any other Backstage plugin that cloud be a Radius Dashboard client but that could change in future. 

## Trust Boundaries

We have a few different trust boundaries for the Dashboard component:

- **Kubernetes Cluster**: The overall environment where the Dashboard component operates and serves clients.
- **Namespaces within the Cluster**: Logical partitions within the cluster to separate and isolate resources and workloads. 

The Dashboard component lives inside the `radius-system` namespace in the Kubernetes cluster where it is installed. UCP also resides within the same namespace.Namespaces within Kubernetes can help set Role-Based Access Control (RBAC) policies.

Also, the dashboard webapp portal is accessible to various configured users.
Users that are signed-in in to Backstage generally have full access to all information and actions. If more fine-grained control is required, the permissions system should be enabled and configured to restrict access as necessary. Summarizing from [Backstage threat model](https://backstage.io/docs/overview/threat-model/), the users could belong to one of these trust levels:

**Internal users** and **Operators**:** are trusted team members who use or maintain an instance of Backstage.

**A builder** is an internal or external code contributor and end up having a similar level of access as operators. 

**An external user** is a user that does not belong to the above two groups, for example a malicious actor outside of the organization. The security model of Backstage currently assumes that this group does not have any direct access to Backstage, and it is the responsibility of each adopter of Backstage to make sure this is the case.

Users should be given access based on the trust level he/she belongs to using the Backstage permissions system.

This web application is not intended to be public-facing; it is available on the intranet for use by both development and operations personnel working on a radified application using kubernetes port-forward
```
kubectl port-forward --namespace=radius-system svc/dashboard 7007:80
``` 

Decisions to make Dashboard public-facing should be the user's conscious choice. 



## Assumptions

This threat model assumes that:

1. The Radius installation is not tampered with.
2. The Kubernetes cluster that Radius is installed on is not compromised.
3. It is the responsibility of the Kubernetes cluster to authenticate users. Administrators and users with sufficient privileges can perform their required tasks. Radius cannot prevent actions taken by an administrator.
4. Dashboard users have been configured to have right level of access by following [Backstage Threat Model](https://backstage.io/docs/overview/threat-model/). These users are trusted to the extent that they are not expected to compromise the availability of Backstage, but they are not trusted to not compromise data confidentiality or integrity.
5. Dashboard is not public facing.
6. Access to Dashboard is using HTTPS.
7. Authentication mechanism provided by Backstage is robust.

## Data Flow

### Diagram

![Radius Dashboard](2024-08-dashboard-component-threat-model/dashboard-tm.png)

1. User types the backstage url and accesses Radius plugin
2. Request reaches the dashboard pod in `radius-system` namespace in kubernetes cluster.
3. The dashboard service sends a Radius API request to UCP.
4. UCP works with ApplicationsCore-RP and sends response back to Dashboard SPA.
5. Dashboard SPA constructs the visuals using components from backstage core, rad-component and data in API response and responds with appropriate page to the user. 

### Threats
 
#### Threat 1: DoS

**Description**

A client can access Dashboard repeatedly or fetch the page in a loop.

**Impact**

Due to the volume of requests Dashboard as well as the UCP, ApplicationsCore-RP components involved in serving the request could run out of resource to serve a legitimate request.
   
**Mitigation**:

Add Radius documentation to capture the below mitigation steps if and when the user chooses to make Backstage available for multiple users.

1. Access to Dashboard should be provided to trusted users.  The [Backstage  permissions system](https://backstage.io/docs/permissions/overview) should be enabled and configured to restrict access as necessary.
   
***Status***:

Active. Operators are expected to configure the system and limit access to Dashboard portal. 

2. Audit logs should be enabled to monitor and report on suspicious user activity. 

***Status***

[In Progress](https://github.com/backstage/backstage/issues/23950)

#### Threat 2: Information Disclosure by unauthorized access to application information

**Description**

Access to app graph through the portal can provide information on dependency between app components.

**Impact**:

A malicious user can utilize the app graph to stage effective attack by targeting a component that has most dependency.

**Mitigation**:

Add Radius documentation to capture the below mitigation steps if and when the user chooses to make Backstage available for multiple users.

1. Access to Dashboard portal should be provided to trusted users. While we do not expose any secrets in db, users should still enable authentication and secure access to data based on roles.
   
***Status***:

Active. Operators are expected to configure the [Backstage  permissions system](https://backstage.io/docs/permissions/overview) to restrict access as necessary. 

2. Audit logs should be enabled to monitor and report on suspicious user activity. 

***Status***

[In Progress](https://github.com/backstage/backstage/issues/23950)

#### Threat 3: Spoofing dashboard service-account can cause DoS 
**Description**

If an unauthorized user or malicious actor tampers with cluster,
he can create services with dashboard service-account to send Radius API requests. 

**Impact**:

A malicious user can utilize Radius API and create too many resources. 

This is because dashboard service-account is allowed get, list, and post requests to UCP

```
metadata:
  name: dashboard
  namespace: {{ .Release.Namespace }}
  labels:
    app.kubernetes.io/name: dashboard
    app.kubernetes.io/part-of: radius
rules:
  - apiGroups: ['api.ucp.dev']
    resources: ['*']
    # dashboard needs get, list, and post privileges for api.ucp.dev
    verbs: ['*']
```

**Mitigation**:

Tampering with the dashboard service-account or creating malicious services would require access to modify the radius-system namespace. Our threat model assumes that the operator has limited access to the radius-system namespace using Kubernetes' existing RBAC mechanism. We should also revisit the threat model and provide a more robust set of authorization controls when granular authorization policies are added to Radius.
   
***Status***:

Active. Operators are expected to secure their cluster and limit access to the radius-system namespace. 

#### Threat 3: Sniffing traffic from dashboard server to browser could result in Information Disclosure. 

If HTTPS is not enabled and the dashboard is made public-facing then a malicious user would be able to see the data used to construct dashboard by sniffing.

**Impact**:

If an unauthorized user or malicious actor gathers data used to construct the page by viewing the network traffic, he can get access to information about application and use it to stage attacks. 

**Mitigation**:

1. Dashboard is not intended to be public-facing. If a decision is taken to make it public, it should be configured to use HTTPS and The [Backstage  permissions system](https://backstage.io/docs/permissions/overview) should be enabled and configured to restrict access as necessary. We should capture this as a necessary step if user chooses to expose Dashboard over internet.
   
***Status***:

Active. Operators are expected to limit access to Dashboard portal. Further, 
they should configure Dashboard to use HTTPS if it is public-facing.

## Open Questions

None

## Action Items


1. We should provide Radius documentation that captures below guidelines to be followed if a customer chooses to allow Dashboard access to multiple users and/ or make dashboard public facing. 

   1. Dashboard should be accessed only on HTTPS if it should be available outside cluster. Currently, we can access the application on http but since we only access the application on localhost using kubernetes port-forward, this is OK. 
    
   2. Enable authentication on Dashboard. This could be tied to RBAC support on Radius, since we might want the same users to be allowed dashboard logins by default with permissions configured using Backstage permission system.

   3. The Backstage permissions system should be enabled and configured to restrict access as necessary. 
   
[#105](https://github.com/radius-project/dashboard/issues/105) logged to capture this effort.
   
1. Revisit threat model when we have RBAC support in Radius and fine tune permissions on Dashboard resources.

## Review Notes

<!--
Update this section with the decisions and feedback from the threat model review meeting. Document any changes made to the model based on the review.
-->

## References

https://backstage.io/docs/overview/threat-model/
https://backstage.io/docs/permissions/overview
