---
type: docs
title: "Radius resource provider settings"
linkTitle: "Resource provider settings"
description: "Settings supported by the Radius resource rrovider"
weight: 30
---

The Radius Resource Provider supports a number of different settings that will configure its behavior. The primary purpose of the resource provider is support Azure integration, so by default the RP behaviors are optimized towards Azure.

Unlike the `rad` CLI or other infrastructure, all of the supported settings for the Radius RP are environment variables.

Many of the optional settings are booleans, which apply the following logic:

- `true` enables the setting. This value is compared *case-insensitively*, so `True` would also be accepted.
- ANY other value disables the setting. 

Enum values are compared *case-insensitively*.

## All settings

| Environment variable           | Required / (default value) | Type    | Description                                                                                                                                  |
| ------------------------------ | -------------------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| PORT                           | **yes**                    | int     | Configures the HTTP listening port of the RP. Must be a valid port number.                                                                   |
| SKIP_AUTH                      | no (false)                 | boolean | Optionally skip authentication checks for inbound HTTP requests to the RP.                                                                   |
| MONGODB_CONNECTION_STRING      | **yes**                    | string  | Configures the connection string of the MongoDB database used to store resources.                                                            |
| MONGODB_DATABASE               | **yes**                    | string  | Configures the name of the MongoDB database used to store resources.                                                                         |
| SKIP_K8S                       | no (false)                 | boolean | Optionally skip connecting to Kubernetes. This means that Kubernetes resources will not be supported.                                        |
| SKIP_ARM                       | no (false)                 | boolean | Optionally skip connecting to ARM. This means that Azure resources will not be supported.                                                    |
| RADIUS_MODEL                   | no (`azure`)               | enum    | Configures the application model to use. This defines the set of support components and output resources. Supported values: `azure`, `k8s`.  |
| ARM_SUBSCRIPTION_ID            | *when ARM is enabled*      | string  | Configures the subscription id used for ARM operations. Falls back to K8S_SUBSCRIPTION_ID when unset.                                        |
| ARM_RESOURCE_GROUP             | *when ARM is enabled*      | string  | Configures the resource group used for ARM operations. Falls back to K8S_RESOURCE_GROUP when unset.                                          |
| AZURE_CLIENT_ID                | no                         | string  | Configures the client id of a service principal for ARM authentication.                                                                      |
| AZURE_CLIENT_SECRET            | no                         | string  | Configures the client secret of a service principal for ARM authentication.                                                                  |
| AZURE_TENANT_ID                | no                         | string  | Configures the AAD tenant of a service principal for ARM authentication.                                                                     |
| MSI_ENDPOINT/IDENTITY_ENDPOINT | no                         | string  | Used to detect whether the RP should use managed identity for ARM authentication.                                                            |
| K8S_LOCAL                      | *see Kubernetes section*   | boolean | Configures the Kubernetes connection to use the local Kubernetes context.                                                                    |
| K8S_CLUSTER                    | *see Kubernetes section*   | boolean | Configures the Kubernetes connection to use the Kubernetes in-cluster config.                                                                |
| K8S_CLUSTER_NAME               | *see Kubernetes section*   | string  | Configures the resource name of an AKS cluster. Used to identify and connect to the Kubernetes cluster by retrieving credentials from ARM.   |
| K8S_SUBSCRIPTION_ID            | *see Kubernetes section*   | string  | Configures the subscription id of an AKS cluster. Used to identify and connect to the Kubernetes cluster by retrieving credentials from ARM. |
| K8S_RESOURCE_GROUP             | *see Kubernetes section*   | string  | Configures the resource group of an AKS cluster. Used to identify and connect to the Kubernetes cluster by retrieving credentials from ARM.  |

## ARM authentication

Authentication with ARM can be disabled totally by setting `SKIP_ARM=true`. This will disable ARM features like creation and management of Azure resources.

The RP can connect to ARM using credentials from one of three different sources in order of priority:

- Service Principal
- Managed Identity (used when deployed)
- CLI authentication (used in local development)

Our detection logic mirrors what the newer Azure Go SDKs do. Since we require the use of the old-style SDKs we also perform the same logic. The environment variables we use to read these settings are the **standard set** used by all Azure tools. eg: `AZURE_CLIENT_ID` is the standard environment variable supported by all Azure tools. 

## Kubernetes

Authentication with Kubernetes can be disabled totally by setting `SKIP_K8S=true`. This will disable Kubernetes features like creation and management of containers/pods and ingresses.

The RP can connect to Kubernetes using three different strategies to find the identity and credentials in order or priority:

- Using local Kubeconfig (used in local development)
- Using in-cluster credentials
- Using ARM to fetch credentials based on `K8S_CLUSTER_NAME`/`K8S_SUBSCRIPTION_ID`/`K8S_RESOURCE_GROUP` (used when deployed)

Our detection logic treats `K8S_LOCAL` and `K8S_CLUSTER` as overrides. We will attempt to find the cluster using ARM by default.