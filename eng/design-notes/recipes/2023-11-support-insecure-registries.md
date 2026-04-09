# Adding Support for publishing recipes to insecure registries

* **Author**: Vishwanath Hiremath (@vishwahiremat)

## Overview

Today we publish/pull recipes to/from the registry that is OCI compliant and only support secure connections i.e registries that are SSL/TLS enabled. And we don't have support to use insecure registry e.g locally hosted k3d registry. And we hit _"http: server gave HTTP response to HTTPS client"_ error if insecure registry is used. This document is centered around addressing this issue.

## Terms and definitions

| Term        | Definition |
| :-- |:--|
| insecure registry      | An insecure registry is the one not having either valid registry certificate or is not using TLS and does not use secure communication protocols like HTTPS. |
| ORAS      | ORAS is the de facto tool for working with OCI Artifacts.https://oras.land/docs/ |
| Recipes      | Recipes enable a separation of concerns between infrastructure operators and developers by automating infrastructure deployment. https://docs.radapp.dev/author-apps/recipes/ |

## Objectives

> **Issue Reference:** https://github.com/radius-project/radius/issues/6648

### Goals

-   Enable the support to publish and pull recipes using a non-secure registry.

### Non goals


### User scenarios (optional)

#### User story 1
As a Radius user I would like to use locally hosted insecure registry to publish bicep recipes.

Scenario: I create a k3d managed cluster running:
```
k3d registry create myregistry.localhost --port 5000
```
And use localhost:5000 as the registry path to publish bicep recipes.
```
rad bicep publish --file ./redis-test.bicep --target br:localhost:5000/myregistry/redis-test:v1
```
I would like to see `rad bicep publish` command successfully publish bicep file to locally hosted registry and also I should be able to read it from the registry during recipe deployment. 


## Design

### Design details
Since we use ORAS client to communicate with the registry, it provides an option "plain-http" to allow insecure connections to registry without SSL check. `plainHttp` property should be set to `true` when client to the remote repository is created to support using insecure registries.

#### Introduce "plain-http" opt-in flag for users.

Users can use `plain-http` flag with `rad bicep publish` command and `plainHttp` property while defining bicep recipes to specify the registry used is insecure and allow connections to registry without SSL check. And based on this property `plainHttp` option is set on the remote repository client. 



### API design (if applicable)

***Model changes***

Addition of optional property `plainHttp` to BicepRecipeProperties (as it's only valid for template kind `bicep`).

environments.tsp
```diff
@doc("Represents Bicep recipe properties.")
model BicepRecipeProperties extends RecipeProperties {
  @doc("The Bicep template kind.")
  templateKind: "bicep";

+  @doc("Allows an insecure connection to a Bicep registry without doing an SSL check. Used commonly for locally-hosted registries. Defaults to false (require SSL).")
+  plainHttp?: boolean;
}

@doc("The properties of a Recipe linked to an Environment.")
model RecipeGetMetadataResponse {
  @doc("The format of the template provided by the recipe. Allowed values: bicep, terraform.")
  templateKind: string;

  @doc("The path to the template provided by the recipe. Currently only link to Azure Container Registry is supported.")
  templatePath: string;

  @doc("The version of the template to deploy. For Terraform recipes using a module registry this is required, but must be omitted for other module sources.")
  templateVersion?: string;

  @doc("The key/value parameters to pass to the recipe template at deployment.")
  parameters: {};

+  @doc("Allows an insecure connection to a Bicep registry without doing an SSL check. Used commonly for locally-hosted registries. Defaults to false (require SSL).")
+  plainHttp?: boolean;
}
```

***CLI changes***

Add plain-http flag to allow insecure connection to a Bicep registry without doing an SSL check.
#### Bicep Publish

```
rad bicep publish --file ./redis-test.bicep --target br:localhost:5000/myregistry/redis-test:v1 --plain-http
```
#### Recipe Register 

```
rad recipe register cosmosdb --template-kind bicep --template-path br:localhost:5000/myregistry/redis-test:v1 --resource-type Applications.Datastores/mongoDatabases --plain-http
```

#### Recipe List/Show
Add a new column to recipe list/show output table.

| NAME        | TYPE           | TEMPLATE KIND  | TEMPLATE VERSION | TEMPLATE |
| :--: |:--:| :--:| :--: | :--:|
| cosmosdb      | Applications.Datastores/mongoDatabases | bicep | | localhost:5000.io/myregistry/redis-test:v1|

***Bicep Changes***

#### Registering recipe through bicep.

Add `plainHttp` property to use insecure registry.
```diff
import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-resources-recipe-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-recipe-env'
    }
    recipes: {
      'Applications.Datastores/mongoDatabases':{
        recipe1: {
          templateKind: 'bicep'
          templatePath: 'ghcr.io/testpublicrecipe/bicep/modules/mongodatabases:v1' 
        }
        recipe2: {
          templateKind: 'bicep'
          templatePath: 'localhost:5000/myregistry/mongo-test:v1' 
+          plainHttp: true
        }
      }
    }
  }
}
```

## Alternatives considered

#### Automatically identify the insecure registry.

Insecure registries are predominantly self-hosted within local environments, so registry url mostly starts with `localhost` or `127.0.0.1` that can be used to identify if `plainHttp` option need to be set on the remote repository client i.e parse the registry URL to check if it starts with `localhost` or `127.0.0.1`.

This could be a quick solution involving minimal changes but it carries various downsides.

- It is not supported for other insecure registries but for locally hosted ones.
- It is not valid if locally managed registry is SSL/TLS enabled.
- Identifying the insecure registry from our end may weaken the security.


## Test plan

#### Unit Tests
-   Update environment conversion unit tests to validate plainHttp property.
-   Add cli unit tests to validate plain-http flag.
-   Update environment controller and config loader tests.

#### Functional tests
- 	Add a e2e test to verify plain-http flag for `rad recipe register` and `rad bicep publish` commands
-   Add a e2e test to deploy a recipe pushed to a locally hosted registry.

## Development plan
- Task 1:  
    - Updating environment typespec, datamodel and conversions.
    - Updating Unit Tests.
- Task 2:
    - Updating environment controller and setting plainHttp flag.
    - Updating controller unit tests.
- Task 3:
    - Adding changed to `rad recipe` and `rad bicep publish` cli.
    - Unit Testing
- Task 4:
    - Adding functional tests.
