# Radius API Guidelines

<!-- markdownlint-disable MD033 MD049 MD055 -->
<!--
Note to contributors: All guidelines have an anchor tag to allow cross-referencing from associated tooling.
The anchor tags within a section using a common prefix to ensure uniqueness with anchor tags in other sections.
Please ensure that you add an anchor tag to any new guidelines that you add and maintain the naming convention.
-->

## History

<details>
  <summary>Expand change history</summary>

| Date        | Notes                                                          |
| ----------- | -------------------------------------------------------------- |
| 2024-Sep-17 | Added guidance on Secrets                                      |

</details>

## Introduction

These are prescriptive guidelines that Radius contributors MUST follow while designing APIs to maintain a great user experience. These guidelines help make Radius APIs developer friendly via consistent patterns.

Technology and software is constantly changing and evolving, and as such, this is intended to be a living document, with a current focus on secrets handling. [Open an issue](https://github.com/radius-project/design-notes/issues) to suggest a change or propose a new idea.

### Prescriptive Guidance
This document offers prescriptive guidance labeled as follows:

:white_check_mark: **YOU MUST** adopt this pattern.

:ballot_box_with_check: **YOU SHOULD** adopt this pattern. If not following this advice, you MUST disclose your reason during a design review discussion.

:heavy_check_mark: **YOU MAY** consider this pattern if appropriate to your situation.

:warning: **YOU SHOULD NOT** adopt this pattern. If not following this advice, you MUST disclose your reason during a design review discussion.

:no_entry: **YOU MUST NOT** adopt this pattern.

## API Foundation:

Radius provides an HTTP-based API that deploys and manages cloud-native applications as well as on-premise or cloud resources.
The overall design of the Radius API is based on the Azure Resource Manager API (ARM). Radius generalizes the design of ARM, removes proprietary Azure concepts, and extends the ARM contract to support the conventions of other resource managers like Kubernetes or AWS Cloud-Control. 

Radius utilizes TypeSpec for describing API model definitions and operations, providing a consistent approach to defining the structure of API payloads and interactions. The TypeSpec specification serves as the source to generate corresponding OpenAPI 2.0 (swagger) API documentation.

Reference: 
- [Radius API](https://docs.radapp.io/concepts/technical/api/)
- [Radius Architecture](https://docs.radapp.io/concepts/technical/architecture/)

## Guidelines

### Resource Property Design

<a href="#secrets" name="secrets"></a>
#### Secrets

<a href="#secret-store" name="secret-store">:white_check_mark:</a> **YOU MUST** use [Applications.Core/secretStores](https://docs.radapp.io/reference/resource-schema/core-schema/secretstore/) resource to store sensitive data, such as passwords, OAuth tokens, and SSH keys. Through the use of this resource, Radius ensures that confidential data is handled in a safe and consistent manner.

The guidance for API design of secrets depends on which of these two scenarios best fits your use case:
- For storing and retrieving a single (scalar) secret value, e.g., an environment variable, refer to [`SecretReference`](#secretreference-model) model type described below.
- When storing and retrieving a structured (multiple-value) secret, e.g., OAuth configuration, refer to [`SecretConfig`](#secretconfig-model) model type described below.

 These prescribed types ensure secure and uniform handling of sensitive information across different components and resources in Radius. Their definitions and usage examples are as follows:
 
##### SecretReference Model

<a href="secretreference-model" name="secretreference-model">:white_check_mark:</a> **YOU MUST** follow this structure when adding support for secrets to resources or components in Radius.

```tsp

@doc("This specifies a reference to a secret. Secrets are encrypted, often have fine-grained access control, auditing and are recommended to be used to hold sensitive data.")
model SecretReference {
  @doc("The ID of an Applications.Core/SecretStore resource containing sensitive data.")
  source: string;

  @doc("The key for the secret in the secret store.")
  key: string;
}

```        

##### Usage of SecretReference

<a href="#secret-envvar" name="secret-envvar">:white_check_mark:</a> **YOU MUST** use the `SecretReference` type to reference a single (scalar) secret value in a resource property. Resource properties should reference a `Applications.Core/secretStores` instead of directly containing secret data.

This pattern simplifies the overall design of Radius by reducing the number of places where we store secret data. 
The structure also allows for environment variables to refer to other resources such as `ConfigMaps`, `Pod Fields` etc. in the future. 
Examples include cases where environment variables containing sensitive information are injected into a container or used in a Terraform execution process.

##### Example TypeSpec Definition

```tsp

@doc("environment")
env?: Record<EnvironmentVariable>;

@doc("Environment variables type")
model EnvironmentVariable {

  @doc("The value of the environment variable")
  value?: string;

  @doc("The reference to the variable")
  valueFrom?: EnvironmentVariableReference;
}

@doc("The reference to the variable")
model EnvironmentVariableReference {
  @doc("The secret reference")
  secretRef: SecretReference;
}

```

##### Example Bicep Definition

```bicep

env: {
  DB_USER: { value: 'DB_USER' }
  DB_PASSWORD: {
    valueFrom: {
      secretRef: {
        source: secret.id
        key: 'DB_PASSWORD'
      }
    } 
  }
} 

```

##### :no_entry: **YOU MUST NOT**: Store sensitive data as clear text
Sensitive data MUST NEVER be stored directly in resource definitions or configuration files as clear text. Instead, use references to secrets stored in secret stores.

##### Example of What NOT to Do

```bicep

credentials: {
  user: 'username'
  password: 'myPlainTextPassword' // ❌ This should be avoided
} 

```

##### SecretConfig Model

<a href="#secretconfig-model" name="secretconfig-model">:white_check_mark:</a> **YOU MUST** implement this model when the structure of the secret store resource is known and a component or resource in Radius requires authentication to external systems, such as private container registries, TLS certificates.

```tsp

@doc("Secret Configuration used to authenticate to external systems.")
model SecretConfig {
  @doc("The ID of an Applications.Core/secretStores resource containing credential information.")
  secret?: string;
}

```        

##### Usage of SecretConfig

<a href="#secretconfig-ext" name="secretconfig-ext">:white_check_mark:</a> **YOU MUST** use the `SecretConfig` type to reference a structured set of secret values in a resource property. Resource properties should reference a `Applications.Core/secretStores` instead of directly containing secret data.

This pattern simplifies the overall design of Radius by reducing the number of places where we store secret data.

##### Example TypeSpec Definition

```tsp

@doc("Authentication information used to access private Terraform modules from Git repository sources.")
model GitAuthConfig {
  @doc("Personal Access Token (PAT) configuration used to authenticate to Git platforms.")
  pat?: Record<SecretConfig>;
}

```

##### Example Bicep Definition

```bicep

recipeConfig: {
  ...
  git: {
    pat: {
      'github.com':{
         secret: secret.id
      }
    }
  }
  ...
}

```