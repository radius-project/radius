# Radius Controller Component Threat Model

- **Author**: ytimocin

## Overview

This document provides a threat model for the Radius Controller component. It identifies potential security threats to this critical part of Radius and suggests possible mitigations. The document includes an analysis of the system, its assets, identified threats, and recommended security measures to protect the system.

The Radius Controller component monitors changes (create, update, delete) in the definitions of Recipe and Deployment resources. Based on these changes, the appropriate controller takes the necessary actions. Below, you will find detailed information about the key parts of the Radius controllers and the objects they manage.

## Terms and Definitions

| Term                  | Definition                                                                                                                                                                                  |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Admission Controllers | An admission controller is a piece of code that intercepts requests to the Kubernetes API server prior to persistence of the object, but after the request is authenticated and authorized. |
| mTLS                  | Mutual Transport Layer Security (mTLS) allows two parties to authenticate each other during the initial connection of an SSL/TLS handshake.                                                 |
| UCPD                  | Universal Control Plane Daemon for Radius                                                                                                                                                   |

## System Description

The Controller component enables users to manage Radius resources through the Kubernetes API. Users define their applications using existing Kubernetes resource types like `Deployment` and/or the `Recipe` CRD provided by Radius. The job of the controller is to ensure that the desired state of the system is maintained in both Radius and Kubernetes by continuously monitoring and reconciling resources.

The Controller component consists of two Kubernetes controllers, Recipe and Deployment controllers, a validating webhook for changes in the Recipe object, and several other important parts. We will dive into more details on the controller below.

Note: If you would like to learn more about Kubernetes controllers, you can visit [this link](https://kubernetes.io/docs/concepts/architecture/controller/).

Note: Kubernetes has [admission controllers](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/) that intercept requests to the Kubernetes API Server before the persistence of the object. Admission controllers may be **validating** (please note that we have a validating webhook in Radius), **mutating**, or both. Here is a simple diagram of the flow from the command entered by the user to the persistence of the object to etcd.

![Admission Controllers' Flow](./2024-08-controller-component-threat-model/admission-controllers-flow.png)

### Architecture

The Controller component consists of several key parts:

- **Recipe and Deployment Controllers**: These are reconciliation loops that watch for changes in Recipe and Deployment resources. These changes can include the addition, update, or deletion of these resources. Whenever one of the reconciliation loops detects a change, it attempts to move the state of the cluster to the desired state.
  - **Recipe Controller**: This reconciliation loop specifically watches for changes in the Recipe resources. After detecting a change, it determines the type of change (addition, update, or deletion) and calls the UCPD to perform the necessary actions.
    - If the change is a create or an update, the Controller calls UCPD to create or update the necessary resources (including secrets).
    - If the change is a delete of a Recipe resource, the Controller calls UCPD to delete the resource and all related resources.
  - **Deployment Controller**: This reconciliation loop specifically watches for changes in the Deployment resources. After detecting a change, it determines the type of change (addition, update, or deletion) and calls the UCPD to perform the necessary actions.
    - If the change is a create or an update, the Controller calls UCPD to create or update the necessary resources (including secrets).
    - If the change is a delete of a Deployment resource, the Controller calls UCPD to delete the resource and all related resources.
- **Recipe Validating Webhook**: This webhook is triggered by the Kubernetes API Server when there is a change (create, update, or delete) in a Recipe resource. The webhook validates the action and responds to the Kubernetes API Server with an approval or a rejection. In our case, the validating webhook only checks if the recipe is one of the Radius portable resources in create or update cases. For more information about webhooks, refer to the [official documentation](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/).
- **Health Checks**: Health checks are implemented to monitor the status and performance of the Controller component as a whole, which includes both controllers and the validating webhook. They ensure that the controllers and the webhook are functioning correctly and can trigger corrective actions if any issues are detected.

### Implementation Details

#### Use of Cryptography

1. **Computing the Hash of the Deployment Configuration**: [Link to code](https://github.com/radius-project/radius/blob/8151a96665b7f5bcd6474f5e33aff35d01adfa5a/pkg/controller/reconciler/annotations.go#L78).

   1. **Purpose**: The purpose of computing the hash of the configuration of the deployment resource is to compare and determine if the deployment is up-to-date or needs an update.
   2. **Library**: The library used to calculate the hash of the deployment configuration is the crypto library, which is one of the standard libraries of Go: [Link to library](https://pkg.go.dev/crypto@go1.23.1).
   3. **Type**: [SHA1](https://www.rfc-editor.org/rfc/rfc3174.html). Note: "SHA-1 is cryptographically broken and should not be used for secure applications." [Link to warning](https://pkg.go.dev/crypto/sha1@go1.23.1). This is used as an optimization for detecting changes, not as a security protection.

2. **Hashing the Secret Data**: [Link to code](https://github.com/radius-project/radius/blob/8151a96665b7f5bcd6474f5e33aff35d01adfa5a/pkg/controller/reconciler/deployment_reconciler.go#L580).

   1. **Purpose**: We hash the secret data and add it to the Pod definition to determine if the secret has changed in an update.
   2. **Library**: The library used to calculate the hash of the secret is the crypto library, which is one of the standard libraries of Go: [Link to library](https://pkg.go.dev/crypto@go1.23.1).
   3. **Type**: [SHA1](https://www.rfc-editor.org/rfc/rfc3174.html). Note: "SHA-1 is cryptographically broken and should not be used for secure applications." [Link to warning](https://pkg.go.dev/crypto/sha1@go1.23.1). This is used as an optimization for detecting changes, not as a security protection.

#### Storage of secrets

Below you will find where and how Radius stores secrets. We create Kubernetes Secret objects and rely on Kubernetes security measures to protect these secrets.

1. **Deployment Reconciler**: Creates or updates a Kubernetes Secret for the connection values in a Deployment object. This Kubernetes Secret is deleted when the Deployment is deleted.
2. **Recipe Reconciler**: Creates a Kubernetes Secret for the Recipe object if it is defined in its spec. If there is an update to the secret object, the old one is deleted and the new one is added. When the Recipe is deleted, the Secret is also deleted.

#### Data Serialization / Formats

We use custom parsers to parse Radius-related resource IDs and do not use any other custom parsers and instead rely on Kubernetes built-in parsers. Therefore, we trust Kubernetes security measures to handle data serialization and formats securely. The custom parser that parses Radius resource IDs has its own security mechanisms that don't accept anything other than a Radius resource ID.

### Clients

In this section, we will discuss the different clients of the Controller component. Clients are systems that interact with the Controller component to trigger actions. Here are the clients of the Controller component:

1. **Kubernetes API Server**: The primary client that interacts with the controller. It communicates with the validating webhook and the controllers in case of resource changes (e.g., creation, update, or deletion of a Recipe or Deployment) requested by another interactor (for example; a human running `kubectl` commands). The controllers watch for these calls and reconcile the state of the resources accordingly.
1. **Health Check Probes**: Kubernetes itself can act as a client by performing health and readiness checks on the controller manager.
1. **Metrics Scrapers**: If metrics are enabled, Prometheus or other monitoring tools can scrape metrics from the controller manager.

## Trust Boundaries

We have a few different trust boundaries for the Controller component:

- **Kubernetes Cluster**: The overall environment where the Controller component operates and receives requests from the clients.
- **Namespaces within the Cluster**: Logical partitions within the cluster to separate and isolate resources and workloads.

The Controller component lives inside the `radius-system` namespace in the Kubernetes cluster where it is installed. UCPD also resides within the same namespace.

The Kubernetes API Server, which is the main interactor of the Controller component, runs in the `kube-system` namespace within the cluster.

### Key Points of Namespaces

1. **Isolation of Resources and Workloads**: Different namespaces separate and isolate resources and workloads within the Kubernetes cluster.
2. **Access Controls and Permissions**: Access controls and other permissions are implemented to manage interactions between namespaces.
3. **Separation of Concerns**: Namespaces support the separation of concerns by allowing different teams or applications to manage their resources independently, reducing the risk of configuration errors and unauthorized changes.

## Assumptions

This threat model assumes that:

1. The Radius installation is not tampered with.
2. The Kubernetes cluster that Radius is installed on is not compromised.
3. It is the responsibility of the Kubernetes cluster to authenticate users. Administrators and users with sufficient privileges can perform their required tasks. Radius cannot prevent actions taken by an administrator.

## Data Flow

### Diagram

![Controller Component via Microsoft Threat Modeling Tool](./2024-08-controller-component-threat-model/controller-component.png)

1. **User creates/updates/deletes a Recipe or a Deployment resource**: When a user requests to create, update, or delete a Recipe or a Deployment resource, the request is handled by the Kubernetes API Server. One way, and probably the most common way, a user can do this request is by running a **kubectl** command. Kubernetes takes care of the authentication and the authorization of the user and its request(s) so we (Radius) don't need to worry about anything here.

2. **Validating Webhook**: The only type of admission controller we have in Radius is the validating webhook for the Recipe resource. The validating webhook ensures that the Recipe object to be created or updated is one of the Radius portable resources. Whenever Kubernetes API Server receives a request to create or update a Recipe object, it communicates the proposed changes with the validating webhook. If the validating webhook validates the changes, then it is persisted to the **etcd** by the Kubernetes API Server.

3. **Recipe and Deployment Reconcilers**: When there is a request to create, update, or delete a Recipe or a Deployment resource, after being validated if the resource is a Recipe resource, the next step is the reconcilation of the resource by the appropriate reconciler. In the Controller component, there are two reconcilers: Recipe and Deployment. These reconcilers are loops that watch the changes in the Recipe and Deployment resources. Whenever there is a change, the reconcilers take the necessary actions to move the actual state to the desired state. These necessary actions include communication with the UCPD to create, update, and/or delete necessary resources.

   1. Communication:
      1. Controller and UCPD:
         1. Poll long-running operations (create, update, or delete) for a Recipe or a Deployment resource.
         2. Create a Radius Resource Group if needed.
         3. Create a Radius Application if needed.
         4. Get a Radius resource like the Environment that is associated with the resource.
         5. Create/Update/Delete a Recipe or a Deployment resource.
         6. Create/Update/Delete a Secret for a Recipe or a Deployment resource.
      2. Controller and the Kubernetes API Server:
         1. Fetch a Recipe or a Deployment object.
         2. Send events related to the operations running.
         3. Update a Recipe or a Deployment object.
         4. Create/Update/Delete a Secret associated with a Recipe or a Deployment object.
         5. List Deployments by filtering them by a specific Recipe object.

### Threats

#### Spoofing UCP API Server Could Cause Information Disclosure and Denial of Service

**Description:** If a malicious actor can spoof the UCP API Server by tampering with the configuration in the Controller, the Controller will start sending requests to the malicious server. The malicious server can capture the traffic, leading to information disclosure. This would effectively disable the Controller, causing a Denial of Service.

**Impact:** All data sent to UCP by the Controller will be available to the malicious actor, including payloads of resources in the applications. The functionality of the Controller for managing resources will be disabled. Users will not be able to deploy updates to their applications.

**Mitigations:**

1. Tampering with the controller code, configuration, or certificates would require access to modify the `radius-system` namespace. Our threat model assumes that the operator has limited access to the `radius-system` namespace using Kubernetes' existing RBAC mechanism.
2. The resource payloads sent to UCP by the Controller do not contain sensitive operational information (e.g., passwords).

**Status:** All mitigations listed are currently active. Operators are expected to secure their cluster and limit access to the `radius-system` namespace.

#### Spoofing the Kubernetes API Server Leading to Escalation of Privilege

**Description:** If a malicious actor could hijack communication between the controller and the Kubernetes API Server, the actor could send requests to the controller. At that point, the controller would be processing illegitimate data.

**Impact:** A malicious actor could use the controllers (Recipe and/or Deployment) to escalate privileges and perform arbitrary operations against Radius/UCP.

**Mitigations:**

1. The controllers authenticate requests to the Kubernetes API Server using credentials managed and rotated by Kubernetes. Our threat model assumes that the API Server and mechanisms like Kubernetes-managed authentication are not compromised.
2. The webhook follows a known Kubernetes implementation pattern and uses widely supported libraries to communicate (client-go, Kubebuilder).
3. Tampering with the controller code, configuration, or authentication tokens would require access to modify the `radius-system` namespace. Our threat model assumes that the operator has limited access to the `radius-system` namespace using Kubernetes' existing RBAC mechanism.

**Status:** All mitigations listed are currently active. Operators are expected to secure their cluster and limit access to the `radius-system` namespace.

#### Spoofing Requests to the Validating Webhook

**Description:** If a malicious actor could circumvent webhook authentication, they could send unauthorized requests to the webhook.

**Impact:** The webhook performs validation only and does not mutate any state. The security impact of spoofing is unclear, but it could potentially lead to unauthorized actions being validated.

**Mitigations:**

1. The webhook authenticates requests (mTLS) from the Kubernetes API Server using a certificate managed and rotated by Kubernetes. Our threat model assumes that the API Server and mechanisms like Kubernetes-managed certificates are not compromised.
2. The webhook follows a known Kubernetes implementation pattern and uses widely supported libraries to implement mTLS (Kubebuilder).
3. Tampering with the webhook code, configuration, or certificates would require access to modify the `radius-system` namespace. Our threat model assumes that the operator has limited access to the `radius-system` namespace using Kubernetes' existing RBAC mechanism.

**Status:** All mitigations listed are currently active. Operators are expected to secure their cluster and limit access to the `radius-system` namespace.

#### Denial of Service Caused by Invalid Request Data

**Description:** If a malicious actor sends a malformed request that triggers unbounded execution on the server.

**Impact:** A malicious actor could cause a denial of service or waste compute resources.

**Mitigations:**

1. The controllers and webhooks use widely supported libraries for all parsing of untrusted data in standard formats.
   1. The Go standard libraries are used for HTTP.
   2. The Kubernetes YAML libraries are used for YAML parsing.
2. Radius/UCP implements a custom parser for resource IDs, a custom string format. This requires fuzz-testing.

**Status:** All mitigations listed are currently active. Operators are expected to secure their cluster and limit access to the `radius-system` namespace.

#### Information Disclosure by Unauthorized Access to Secrets

**Description:** A malicious actor could circumvent Kubernetes RBAC controls and gain unauthorized access to Kubernetes secrets managed by Radius. These secrets may contain sensitive information, such as credentials intended for use by applications.

**Impact:** A malicious actor could gain access to sensitive information.

**Mitigations:**

1. Secret data managed by the controllers is stored at rest in Kubernetes secrets. Our threat model assumes that the API server and mechanisms like Kubernetes authentication/RBAC are not compromised.
2. Secrets managed by Radius are always placed in the same namespace as the object that "owns" them. This is a requirement of the Kubernetes RBAC model.
3. Secrets managed by Radius are subject to the Kubernetes RBAC model for controlling access. Operators are expected to limit access for users using existing tools.

**Status:** All mitigations listed are currently active. Operators are expected to secure their cluster and limit access for users.

#### Escalation of Privilege by Using Radius to Circumvent Kubernetes RBAC Controls

**Description:** A malicious actor could circumvent Kubernetes RBAC controls and create arbitrary resources in Kubernetes by using the `Recipe` custom resource.

The `Recipe` controller has limited permissions, so it cannot be used directly to escalate privileges in Kubernetes. However, it calls into UCP/Radius, which operates with a wide scope of permissions in Kubernetes and the cloud.

Authorized users with access to create a `Recipe` resource in Kubernetes can execute any Recipe in any Environment registered with Radius.

At the time of writing, Radius does not provide granular authorization controls. Any authenticated client can create any Radius resource and execute any action Radius is capable of taking. This is not limited to the Kubernetes controllers.

**Impact:** An authorized user of the Kubernetes cluster with permission to create a `Recipe` resource can execute any Recipe in any Environment registered with Radius.

**Mitigations:**

1. Operators should limit access to the `Recipe` resource using Kubernetes RBAC.
2. Operators should limit direct access to the Radius API using Kubernetes RBAC.
3. We should revisit the threat model and provide a more robust set of authorization controls when granular authorization policies are added to Radius.

**Status:** These mitigations are partial and require configuration by the operator. We will revisit and improve this area in the future.

## Open Questions

## Action Items

1. Use a hashing algorithm other than SHA-1 while computing the hash of the configuration of a Deployment object. This is a breaking change because deployments that are already hashed with SHA1 should be redeployed so that reconciler can work as expected.
2. Check if RBAC with Least Privilege is configured for every component to ensure that each component has only the permissions it needs to function. Make changes to the necessary components if required.
3. Define and implement necessary Network Policies to ensure that communication is accepted only from expected and authorized components. Regularly review and update these policies to maintain security.
4. Containers should run as a non-root user wherever possible to minimize the risks. Check if we can run any of the Radius containers as non-root. Do the necessary updates.

## Review Notes

## References

1. <https://kubernetes.io/blog/2018/07/18/11-ways-not-to-get-hacked>
2. <https://www.rfc-editor.org/rfc/rfc3174.html>
3. <https://pkg.go.dev/crypto/sha1@go1.23.1>
