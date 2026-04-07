# Adding support for Terraform Modules from Private Git Repository

* **Author**: Vishwanath Hiremath (@vishwahiremat)

## Overview

Today, radius can work with publicly-hosted Terraform modules across a few different module sources. We don't support working with privately-hosted Terraform modules, and there's no way to configure authentication.

This is important because organizations write their own Terraform modules and store them in privately-accessible sources. In order for us to support serious use, we need to enable private sources.

## Terms and definitions
| Term     | Definition                                                                                                                                                                                                 |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Terraform module source | The source argument in a module block tells Terraform where to find the source code for the desired child module |
| Module registry | A module registry is the native way of distributing Terraform modules for use across multiple configurations |
| Terraform registry | Terraform Registry is an index of modules shared publicly |
| HTTP URLs | When you use an HTTP or HTTPS URL, Terraform will make a GET request to the given URL, which can return another source address | 
| Private Terraform Repository | A private Terraform repository typically refers to a version control repository that contains Terraform module code, but is not publicly accessible. | 


## Objectives

> **Issue Reference:** https://github.com/radius-project/radius/issues/6911

### Goals
- Enable support to use terraform modules from private Git repositories.
- It should support git from any platforms(like Bitbucket, Gitlab, Azure DevOps Git etc.)

### Non goals
- To Support other terraform module sources like S3, GCP, Mercurial repository. 
Support for other sources will be implemented in future iterations.
- To support recipes distributed across multiple private Git repositories in an environment.

### User scenarios (optional)

As an operator I am responsible for maintaining Terraform recipes for use with Radius. Terraform modules used contains sensitive information, proprietary configurations, or data that should not be shared publicly and its intended for internal use within our organization and have granular control over who can access, contribute to, or modify Terraform modules. So I store the terraform modules in a private git repository. And I would like to use these github sources from a private repository as template-path while registering a terraform recipe.

## Design
### Design details
Today, we support only Terraform registry and HTTP URLs as allowed module sources for Terraform recipe template paths. Git public repository are also supported by providing the HTTP URL to the module directory. But to support repositories we need a way to authenticate the git account where the modules are stored.

Git provides different ways to authenticate:

#### Personal Access Token (Proposed option):
Users need to a Git personal access token with very limited access (just read access) and also specify token validity. And this is used along with the username to clone the terraform module repository through HTTPS as part of `terraform init`.

#### SSH key:
SSH key can be used to provide access to private git repository. But it requires adding the generated ssh key to the users account.

#### Service Principal(Only for Azure DevOps Git):
We could use the Azure Service Principal details used for Azure scope to authenticate Azure DevOps Git. But most often users have diff tenant IDs for the production environment and for git.


Generic Git Repository URL format:
```diff
resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-resources-recipe-env'
  location: location
  properties: {
    ...,
    recipes: {
      'Applications.Datastores/mongoDatabases':{
        recipe1: {
          templateKind: 'terraform'
+          templatePath: "git::https://{username}:{PERSONAL_ACCESS_TOKEN}@example.com.com/test-private-repo.git"
        }
      }
    }
  }
}
```
this takes latest as the default version if not provided. To specify a particular version the URL should be appended with `ref=<version>"`
E.g:
```
templatePath: "git::https://{username}:{PERSONAL_ACCESS_TOKEN}@example.com.com/test-private-repo.git?ref=v1.2.0"
```

Since adding sensitive information like tokens to the terraform configuration files which may be stored in version control can pose security issues. So we could use different ways to store the git credentials.

#### Using Applications.Core/secretStores to store private repository credentials


Use secretStore to store the username and personal access token and add the secret to the new property recipeConfig in the environment.

```diff
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
+   recipeConfig: {
+     terraform: {
+       authentication:{
+         git:{  
+           pat:{
+             "github.com":{
+               secret: secretStoreGithub.id
+             },
+             "dev.azure.com": {
+               secret: secretStoreAzureDevOps.id
+             },
+           }
+         }          
+       }
+     }
+   }
    recipes: {      
      'Applications.Datastores/mongoDatabases':{
        default: {
          templateKind: 'terraform'
          templatePath: 'https://dev.azure.com/test-private-repo'
        }
      }
    }
  }
}

resource secretStore 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'github'
  properties:{
-  	app: app.id
    type: 'generic'
    data: {
      'pat': {
        value: '<personal-access-token>'
      }
      'username': {
        value: '<username>'
      }
    }
  }
}

```
E.g of secretstore using the existing kubernetes secret.
```diff
resource existingSecret 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'existing-secret'
  properties:{
-  	app: app.id
    resource: '<namespace>/<existing-secret-name>' // Reference to the name of a an external secret store
    type: 'generic' // The type of secret in your resource
    data: {
      // The keys in this object are the names of the secrets in an external secret store
      'tls.pat': {}
      'tls.username': {}
    }
  }
}
```
SecretStore resource also provides an option to use the existing secret, which makes it better way store credentials. But today's secret store implementation is tied to application scope and in this case, secretStore needs to be created before application and environment creation. So we need to change scope of secret store resource to global (by removing the required flag for application property).

And the referenced secret is retrieved from the secretStore and is added to the git config file as a URL config as shown below. While deploying terraform recipe if it encounters the domain name of the module source, it automatically picks the credential information from the git config file.

```diff
+ git config --global url."https://{username}:{PERSONAL_ACCESS_TOKEN}@github.com".insteadOf https://github.com
```
But we cannot use the global git config as different environments can have terraform recipes coming from different github accounts which leads to race condition. To overcome this issue we could create a local config inside terraform working directory and add conditional paths in the git global config to point to the local config.


```diff
+ // Initialising git inside terraform working directory.
+ // Cannot run "git config --file" command without having .git folder inside terraform working directory.
+
+ git init
+
+ // Adding the git credentials to local git config file created by git init in the previous step.
+
+ git config --file <terraform_working_directory> url."https://{username}:{PERSONAL_ACCESS_TOKEN}@github.com".insteadOf https://github.com
+
+// Add conditional git directory path in global config
+ git config --global includeIf."gitdir:<terraform working directory>/".path <terraform working directory>/.git/config
```
Conditional path config in the global git config is unset after the terraform recipe operation.
```diff
+// Unset conditional git directory path in global config
+ git config --global --unset includeIf."gitdir:<terraform working directory>/".path
```
***Sequence Diagram***

![Alt text](./2024-01-support-private-terraform-repository/sequence_diagram.png)

### API design (if applicable)
***Model changes***

Addition of new property `recipeConfig` to environment properties.

```diff
@doc("Environment properties")
model EnvironmentProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("The compute resource used by application environment.")
  compute: EnvironmentCompute;

  @doc("Cloud providers configuration for the environment.")
  providers?: Providers;

  @doc("Simulated environment.")
  simulated?: boolean;

  @doc("Specifies Recipes linked to the Environment.")
  recipes?: RecipeProperties;

+  @doc("Specifies recipe configurations needed for the recipes.")
+  recipeConfig?: RecipeConfigProperties;

  @doc("The environment extension.")
  @extension("x-ms-identifiers", [])
  extensions?: Array<Extension>;
}

+model RecipeConfigProperties {
+  @doc(Specifies the terraform config properties)
+  terraform?: TerraformConfigProperties;
+}

+model TerraformConfigProperties{
+  @doc(Specifies authentication information needed to use private terraform module repositories.)  
+  authentication?: AuthConfig
+}

+model AuthConfig{
+  @doc("Specifies authentication information needed to use private terraform module repositories from git module source")  
+  git?: GitAuthConfig
+}

+model GitAuthConfig{
+  @doc("Specifies the secret details of type personal access token for each different git platforms")  
+  pat?: Record<Secret>
+}

+model Secret {
+  @doc("The resource id for the secret containing credentials for git.")
+  secret?: string;
+}
```

## Alternatives considered

Here are couple of different designs considered for storing secrets.

### Saving it as part of the environment resource.

Add a new property `recipeConfig` write-only to store the information about the git credentials, and have a custom action `getRecipeConfiglike` (like `list-secrets`) to get these details. 

```diff
+@secure()
+param username string
+@secure()
+param token string
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
+   recipeConfig: {
+     terraform: {
+       gitCredentials: {
+         "dev.azure.com": {
+            username : username
+            pat: token
+          }
+       }
+     }
+   }
    recipes: {      
      'Applications.Datastores/mongoDatabases':{
        default: {
          templateKind: 'terraform'
          templatePath: 'https://dev.azure.com/test-private-repo'
        }
      }
    }
  }
}
```
But the majority of users frequently update their git personal access tokens (like once a day), and which means updating the environment resource every time the token is updated, also this raises potentials security issues storing the these details as part of environment. 

### Using kubernetes secret

Using existing kubernetes secret i.e asking the users to have the git credentials already stored in the kubernetes secret on the same cluster and use it in the recipeConfig.

```diff
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
+   recipeConfig: {
+     terraform: {
+       gitCredentials: {
+         "dev.azure.com": {
+            secret: secretStore.id
+            namespace: <namespace>
+          }
+       }
+     }
+   }
    recipes: {      
      'Applications.Datastores/mongoDatabases':{
        default: {
          templateKind: 'terraform'
          templatePath: 'https://dev.azure.com/test-private-repo'
        }
      }
    }
  }
}
```

## Test plan
#### Unit Tests
-   Update environment conversion unit tests to validate recipeConfig property.
-   Update environment controller unit tests to add recipe config.
-   Adding new unit tests in terraform driver validating recipe config changes and retrieving secrets.

#### Functional tests
- Add e2e test to verify recipe deployment using a terraform module stored in a private git repository.

## Development plan
- Task 1:  
    - Update Application.Core/secretStores to be a global scoped resource.
    - Update unit tests and validating it doesn't break the existing functionality.
- Task 2:
    - Adding a new property recipeConfig to environment and making necessary changes to typespec, datamodel and conversions.
    - Updating unit tests.
- Task 3:
    - Adding environment controller changes.
    - Adding changes to terraform driver to retrieve secrets add it to template path.
    - Update/Add unit tests
- Task 4:
    - Manual Validation and adding e2e tests to verify using private terraform repositories


