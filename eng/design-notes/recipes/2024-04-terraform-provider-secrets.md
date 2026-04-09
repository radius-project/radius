# Handling of Secrets Data for Terraform Providers

* **Author**: @lakshmimsft

## Overview

As part our effort to support multiple Terraform Providers in Radius we're enabling users to input sensitive data into the provider configurations and environment variables using secrets.
This document describes in detail the handling of secrets data between the Engine and Driver for Terraform Providers.

References:
[Design Document to support multiple Terraform Providers](https://github.com/radius-project/design-notes/blob/main/recipe/2024-02-terraform-providers.md).
[Design document for Private Terraform Repository](https://github.com/radius-project/design-notes/blob/main/recipe/2024-01-support-private-terraform-repository.md)

## Terms and definitions

| Term     | Definition                                                                                                                                                                                                 |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Terraform Provider | A Terraform provider is a plugin that allows Terraform to interact with and manage resources of a specific infrastructure platform or service, such as AWS, Azure, or Google Cloud. |

## Objectives

> **Issue Reference:** <!-- (If appropriate) Reference an existing issue that describes the feature or bug. -->
https://github.com/radius-project/radius/issues/6539

### Goals
* Enable users to input data stored in secrets into Terraform provider configurations and run Terraform recipes.
* The secrets will be stored in `Applications.Core/secretStores` underlying resource for the secret store (which today supports Kubernetes secrets).

### Non goals
 Other source and types of `secretsStores` apart from `Applications.Core/secretStores` are out of scope for the current design.
 There is an open issue to expand use-cases of Applications.Core/Secrets [here](https://github.com/radius-project/radius/issues/5520).

### User scenarios (optional)

#### User story 1
As an operator, I maintain a set of Terraform recipes for use with Radius. I have a set of provider configurations which are applicable across multiple recipes. These provider configurations now include secrets, which are injected into the Terraform configuration, allowing the Terraform process to access them as needed.

## User Experience (if applicable)

**Sample Input:**
 
``` diff
resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'dsrp-resources-env-recipes-context-env'
  location: 'global'
  properties: {
    compute: {
      ...  
    }
    providers: {
      ...
    }
    recipeConfig: {
      terraform: {
        authentication: {
          ...
        }
        providers: {
          azurerm: [
            {
              subscriptionid: 1234
              tenant_id: '745fg88bf-86f1-41af'
+              secrets: { // Individual Secrets from SecretStore
+                my_secret_1: {
+                  source: secretStoreConfig.id
+                  key: 'secret.one'
+                }
+                my_secret_2: {
+                  source: secretStoreConfig.id
+                  key: 'secret.two'
+                }
+              }
            }, {
              subscriptionid: 1234
              tenant_id: '745fg88bf-86f1-41af'
              alias: 'az-paymentservice'
            } ]
          gcp: [
            {
              project: 1234
              regions: [ 'us-east1', 'us-west1' ]
            }
          ]
          oraclepass: [
            {
              database_endpoint: '...'
              java_endpoint: '...'
              mysql_endpoint: '...'
            }
          ]
        }
      }
      env: {
        MY_ENV_VAR_1: 'my_value'
      }
+      envSecrets: { // Individual Secrets from SecretStore
+        MY_ENV_VAR_2: {
+          source: secretStoreConfig.id
+          key: 'envsecret.one'
+        }
+        MY_ENV_VAR_3: {
+          source: secretStoreConfig.id
+          key: 'envsecret.two'
+        }
+     }
    }
    recipes: {
    }
  }
}

```

**Sample Output:**
n/a

**Sample Recipe Contract:**
n/a

## Design

### High Level Design

The design for handling secrets for Private Terraform modules established the basis for data flow between Engine and Driver components in the codebase. For Terraform Providers, we will continue to maintain the flow where secrets are retrieved in the Engine and passed into the Driver in the Execute() call. 
The following diagram describes the updated data flow. The calls in green highlight updates to existing code.

![alt text](2024-04-terraform-provider-secrets.png)

### Detailed Design

The FindSecretIds function in the DriverWithSecrets interface is updated to return a map of SecretStoreIds and keys. These will hold data for a Terraform module as before and in addition, also hold SecretStoreIds and keys for provider configuration and environment variables, iterating through the input environment recipe configuration. 

The LoadSecrets method in the SecretLoader interface will be updated return secret data for multiple Ids returned from FindSecretIds().

Note: There can be multiple secrets registered across different providers, but a particular recipe deployment may not require all of them. With the current design we are going to pass in data for all secrets for all providers to the driver and write the provider configurations to Terraform configuration file. 
We have an open design question for tracking potential future changes to this design.

#### Advantages (of each option considered)
Retrieving secrets in the Engine and passing them into the Driver was discussed positively as a way forward. Looking ahead with Containerization, the Driver will have all the information it needs to be able to be spin off into a separate process.

#### Disadvantages (of each option considered)
N/A

#### Proposed Option
N/A

### API design (if applicable)
N/A

### CLI Design (if applicable)
N/A

### Implementation Details
### Recipes (Engine and Driver)

Driver Updates:
```go
//*** CURRENT ***//

type DriverWithSecrets interface {
  ...
	// FindSecretIDs gets the secret store resource ID references associated with git private terraform repository source.
	// In the future it will be extended to get secret references for provider secrets.
	FindSecretIDs(ctx context.Context, config recipes.Configuration, definition recipes.EnvironmentDefinition) (string, error)
}

//*** NEW ***//

// FindSecretIDs will be updated in current DriverWithSecrets interface to now return map of secretStoreIds and keys
type DriverWithSecrets interface {
  ...
	// FindSecretIDs gets the secretStore resource IDs and keys for the recipe including module, provider, envSecrets.
	FindSecretIDs(ctx context.Context, config recipes.Configuration, definition recipes.EnvironmentDefinition) (secretIDs map[string][]string, error) // secretIDs is a map of IDs of the secret stores, and the values are slices of the keys in corresponding secret store.
}
```

Engine Updates

```go
//*** CURRENT ***//

type SecretsLoader interface {
	LoadSecrets(ctx context.Context, secretStore string) (v20231001preview.SecretStoresClientListSecretsResponse, error)
}

// BaseOptions is the base options for the driver operations.
type BaseOptions struct {
  ...

	// Secrets specifies the module authentication information stored in the secret store.
	Secrets v20231001preview.SecretStoresClientListSecretsResponse
}


//*** NEW ***//

// LoadSecrets function in SecretsLoader interface will populate resolved secrets based on the Ids returned from FindSecretIDs(). The result is passed as BaseOptions.Secrets to the Driver in Execute() call.
type SecretsLoader interface {
	LoadSecrets(secretIds map[string][]string, ...)(secretData map[string]map[string]string, error) // secretData is a map of secretStoreID to map of [secretKey]value
}

// When calling driver.Execute we pass in BaseOptions which is structured as:
type BaseOptions struct {
   ...

	// Secrets specifies the module authentication information stored in the secret store.
	Secrets map[string]map[string]string //Secrets is a map of secretStoreID to map of [secretKey]value
}
```

### Error Handling

If there is an error when iterating through the recipe configuration or calling ListSecrets API, error will be returned. These errors will not include sensitive data on secret keys or values.

## Test plan

Testing will include unit tests on all new functions created and a
functional Test will be created to test this functionality e2e.\
Unit Tests should include test for 'secret not found'
and verify that upon returning an error, we do not return sensitive information in the error message.

## Security

Secret data will be held in memory and be passed to the Driver which will inject these values into the Terraform configuration in working directory for Terraform. This directory is created when a Terraform recipe is set to be deployed and deleted once deployment of the Terraform recipe is completed.
Secret data will not be persisted and secret data will not be logged by Radius.

## Compatibility (optional)
N/A

## Monitoring and Logging

No updates to existing metrics and logs. 
We need to make sure we do not log sensitive information within calls.

## Development plan
Updates to Engine, Driver function call - (under 0.5 Sprint)
Unit Tests + Functional Test - (under 0.5 Sprint)

Total: 1 Sprint

## Open Questions
N/A

## Alternatives considered
We could also update FindSecretIds() to return a list of SecretStoreIds and the LoadSecrets function retrieves all secrets for the SecretStore and passes the data to the driver.
We decided to limit the secret data sent to the Driver to only what the Driver will need and are going with the current design. This can be updated in the future if needed.


## Design Review Notes
1. Update design per discussion.Design can be more generic and need not follow Env ConfigStructure.
Earlier design included creating two new constructs to store RecipeSecretIds{} used to call the ListSecrets API and RecipeSecrets{} to store result of resolved Secret data. These closely mimicked the structure of environment recipe configuration structure. Based on design discussions, we simplified the function calls to be more generic and dropped use of the new constructs. 

2. With the current design we configure Terraform providers at the environment level. During a recipe execution, secret data will be processed for all provider configurations and passed to the Driver. During design discussions, it was brought up that it's possible for a specific recipe execution to not need all providers configured and therefore all data sent across and it led to possibility of a more granular approach - recipe specific provider configuration versus the current environment level provider configuration.
We were looking to get some more input from PMs after discussions with clients on their scenarios. Per PM feedback, this is currently not being requested but can be revisited based on future input from clients.


