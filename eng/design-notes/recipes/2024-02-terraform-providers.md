# Support multiple Terraform Providers

* **Author**: @lakshmimsft

## Overview

As part of Terraform support in Radius, we currently support azurerm, kubernetes and aws providers when creating recipes. We pull credentials stored in UCP and setup and save provider configuration which is accessible to recipes.\
This document  describes a proposal to support multiple providers, setup their provider configuration, which would be accessible to all recipes within an  environment.

## Terms and definitions

| Term     | Definition                                                                                                                                                                                                 |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Terraform Provider | A Terraform provider is a plugin that allows Terraform to interact with and manage resources of a specific infrastructure platform or service, such as AWS, Azure, or Google Cloud. |
| alias | alias is a meta-argument that is defined by Terraform and available for all provider blocks. It is used when defining multiple configurations for the same provider and helps the user identify which configuration to use on a per-resource or per-module basis. Ref: [link](https://developer.hashicorp.com/terraform/language/providers/configuration#alias-multiple-provider-configurations)|


## Objectives

**Reference for new type recipeConfig and handling of secrets:** [Design document for Private Terraform Repository](https://github.com/radius-project/design-notes/blob/3644b754152edc97e641f537b20cf3d87a386c43/recipe/2024-01-support-private-terraform-repository.md)

> **Issue Reference:** <!-- (If appropriate) Reference an existing issue that describes the feature or bug. -->
https://github.com/radius-project/radius/issues/6539

### Goals

1. Enable users to use terraform recipes with multiple providers (including and outside of Azure, Kubernetes, AWS).
2. Enable users to use terraform recipes with multiple configurations for the same provider using *alias*. 
3. Enable provider configurations to be available for all recipes used in an environment.


### Non goals
1. Updates to Bicep Provider configuration. The focus of this effort is targeted for Terraform provider support. Bicep provider support will be addressed as a separate initiative as warranted.
2. Authentication for providers hosted in private registries. This is out of scope for the current design effort and will be addressed in the future as required.
3. Support for .terraformrc files. These are CLI configuration files and not in scope of current effort to support multiple provider configuration to be used by recipes. ref: [link](https://developer.hashicorp.com/terraform/cli/config/config-file)

### User scenarios (optional)

#### User story 1
As an operator, I maintain a set of Terraform recipes for use with Radius. I have a set of provider configurations which are applicable across multiple recipes and I would like to configure them in a single centralized location for ease of maintenance.


#### User story 2
As an operator, I would like to manage cloud resources in different geographical regions. To enable this, I need to set up multiple configurations for the same provider for different regions and use an alias to refer to individual provider configurations. 

## Design
### Design details

Reviewing popular provider configurations (GCP, Oracle, Heroku, DigitalOcean, Docker etc.) a large number of provider configurations can be setup by handling a combination of key value pairs, key value pairs in nested objects, secrets and environment variables.

### Key-value pairs
Attributes or settings, represented as key-value pairs are often used to define configuration parameters in provider configurations.
We will parse and save these values in the Terraform configuration file, main.tf in a working directory, which the system creates today. 

### Alias

Aliases in Terraform allow users to manage multiple instances of the same provider with different configurations for a single module. 
Ref links: 
- [Alias Multiple Provider Configurations](https://developer.hashicorp.com/terraform/language/providers/configuration#alias-multiple-provider-configurations)
- [Passing Providers Explicitly](https://developer.hashicorp.com/terraform/language/modules/develop/providers#passing-providers-explicitly)

For a Terraform recipe, aliases needed by the module are declared in the required_providers block, where each alias is a unique provider configuration.
eg.

```
terraform {
  required_providers {
    mycloud = {
      source  = "mycorp/mycloud"
      version = "~> 1.0"
      configuration_aliases = [ mycloud.west_region, mycloud.east_region ]
    }
  }
}
// The `required_providers` block for this module specifies that it will need provider configuration for aliases west_region and east_region to deploy successfully.
```

The bicep configuration for Terraform providers described in the environment resource is typically written to `provider` block in the main.tf.json file. These provider configurations are then passed to the recipe module. For provider configurations without aliases, they are be passed to the module automatically by Terraform.

However, when the recipe module specifies one or more aliases in the `required_providers` block (and the module does not contain the provider configuration details), the aliased provider configuration from the `provider` block needs to be passed explicitly to the module.

The provider alias mapping will be added into the module block of the main.tf.json file through a code update. Note that the provider configurations in Bicep and the recipe module, which includes the `required_providers` blocks, are authored by the user and transcribed into the .main.tf.json file without updates. The module block, is generated by the Radius code.


#### Use Case 1: Matching aliases between provider configuration and `required_provider` block: sample generated main.tf.json file

With current design, we are able to update the module configuration block with provider aliases when a match is found.

```json
{
    "terraform": {
        "required_providers": {
            "kubernetes": {
                "source": "hashicorp/kubernetes",
                "version": ">= 2.0"
            },
            "postgresql": {
                "source": "cyrilgdn/postgresql",
                "version": "1.16.0",
                "configuration_aliases": [
                    "postgresql.pgdb-test"
                ]
            }
        }
    },
    "provider": {
       "postgresql": [
          {
          "alias"    : "pgdb-test",
          "host"     : "postgres.corerp-app.svc.cluster.local",
          "port"     : 5432,
          "username" : "postgresuser",
          "password" : "*********",
          "sslmode"  : "disable"
          }
       ],
        "azurerm": [
          {
            "alias": "az_central",
            "subscriptionid": 1234,
            "tenant_id": "sample_tenant_id"
          }
        ]
    },
    "module": {
        "defaultpostgres": {
        //... 
        "providers": {
          "postgresql.pgdb-test": "postgresql.pgdb-test"
        }
      }
    }
}
```

#### Use Case 2: Terraform configuration allows passing of provider configurations with aliases different from those declared in `required_providers` block to the recipe module.

In order to support this use case we will have to delve further to design how we enable users to map aliases in provider configuration to those declared in recipe modules in `required_providers` blocks across different recipes.

Before proceeding ahead, it would be beneficial to gather more information about user scenarios to ensure we are addressing their needs effectively.

example:

```json
{
    "terraform": {
        "required_providers": {
            "aws": {
                "source": "hashicorp/aws",
                "version": ">= 3.0",
                "configuration_aliases": [
                    "aws.test1", "aws.test2"
                ]
            }
        }
    },
    "provider": {
        "aws": [
            {
                "alias": "aws-west",
                "region": "us-west-1",
                "access_key": "sample_access_key",
                "secret_key": "sample_secret_key"
            },
            {
                "alias": "aws-central",
                "region": "us-central-1",
                "access_key": "sample_access_key",
                "secret_key": "sample_secret_key"
            }
        ]
    },
    "module": {
        "examplemodule": {
            // ...
            "providers": {
                "aws.test1": "aws.aws-west",       
                "aws.test2": "aws.aws-central"
            }
        }
    }
}

```

#### Decision after Design meeting: 
We will proceed with implementing Use Case 1 at this time. This will accommodate for a large portion of user needs. We will revisit Use Case 2 in the future, based on user input and feedback.

### Secrets
Secrets will be handled similarly to the approach described in document [Design document for Private Terraform Repository](https://github.com/radius-project/design-notes/blob/3644b754152edc97e641f537b20cf3d87a386c43/recipe/2024-01-support-private-terraform-repository.md) wherein Applications.Core/secretStores can point to an existing K8s secret.

The system will call the ListSecrets() api in Applications.Core namespace, retrieve contents of the secret and build the Terraform provider configuration.

<u>Update</u>: Please refer to [PR link](https://github.com/radius-project/design-notes/pull/39/files) for design document on implementing Secrets for Terraform Providers.

### Environment variables
In a significant number of providers, as per documentation, environment variables are used one of the methods of saving sensitive credential data along with insensitive data. We allow the users to set environment variables for provider configuration. For sensitive information, we recommend the users save these values as secrets and point to them inside the env block.

```
...
 recipeConfig: {
    terraform: {
       ...
       providers: [...]
    env: {
      'MY_ENV_VAR_1': 'my_value'
    }   
    envSecrets: {                       // Individual Secrets from SecretStore
      'MY_ENV_VAR_2': {
          source: secretStoreConfig.id
          key: 'envsecret.one'
      }
      'MY_ENV_VAR_3': {
        source: secretStoreConfig.id
        key: 'envsecret.two'
      }
    }
  }
 }  
```

Environment variables apply to all providers configured in the environment. The system cannot set two separate values for the same environment variable for multiple provider configurations. In such cases, per provider documentation, there may be alternatives that users can avail (eg. For GCP, users can set  *credentials* field inside each instance of provider config as opposed to using env variable GOOGLE_APPLICATION_CREDENTIALS).


The system will allow for ability to set up multiple configurations for the same provider using the keyword *alias*. Validation for configuration 'correctness' will be handled by Terraform with calls to terraform init and terraform apply.

```
...
 recipeConfig: {
    terraform: {
      ...
      providers: [
      {
        name: 'azurerm',
        properties: {
          subscriptionid: 1234,
          secrets: {                  // Individual Secrets from SecretStore
            'my_secret_1': {
              source: secretStoreAz.id
              key: 'secret.one'
            }
            'my_secret_2': {
               source: secretStoreAzPayment.id
              key: 'secret.two'
            }
          }
        }
      },
      {
        name: 'azurerm',
        properties: {
          subscriptionid: 1234,
          alias: 'az-paymentservice'
       }
     }]
...       
```
Configuration for providers as described in this document will take precedence over provider credentials stored in UCP (currently these include azurerm, aws, kubernetes providers). So, for eg. In the scenario where credentials for Azure are saved with UCP during Radius install and a Terraform recipe created by an operator declares 'azurerm' under the the *required_providers* block; If there exists a provider configuration under the *providers* block under *recipeConfig*, these would take precedence and used to build the Terraform configuration file instead of Azure credentials stored in UCP. 


### Example Bicep Input :
**Option 1**: 

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
        authentication:{
          ...        
        }
+       providers: [
+         {
+            name: 'azurerm',
+            properties: {          
+             subscriptionid: 1234,
+             secrets: {                  // Individual Secrets from SecretStore
+               'my_secret_1': {
+                  source: secretStoreAz.id
+                  key: 'secret.one'
+                }
+               'my_secret_2': {
+                  source: secretStoreAzPayment.id
+                  key: 'secret.two'
+               }
+             }
+            }
+          },
+          {
+            name: 'azurerm',
+            properties: {
+              subscriptionid: 1234,
+              tenant_id: '745fg88bf-86f1-41af-'
+              alias: 'az-paymentservice',
+            }
+          },
+          {
+            name: 'gcp',
+            properties: {
+              project: 1234,
+              regions: ['us-east1', 'us-west1']
+            }
+          },
+          {
+            name: 'oraclepass',
+            properties: {
+              database_endpoint: "...",
+              java_endpoint: "...",
+              mysql_endpoint: "..."
+            }
+          }
+        ]
+      }
+      env: {
+        'MY_ENV_VAR_1': 'my_value'
+        secrets: {                       // Individual Secrets from SecretStore
+        'MY_ENV_VAR_2': {
+            source: secretStoreConfig.id
+            key: 'envsecret.one'
+          }
+        'MY_ENV_VAR_3': {
+            source: secretStoreConfig.id
+            key: 'envsecret.two'
+          }
+        }
+      }      
+    }
  }
  recipes: {      
    ...
  }
}
```
**Option 2**:
This option provides grouping and efficiency to retrieve all details for a single provider. 
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
        authentication:{
          ...        
        }
+       providers: {
+         'azurerm': [
+           {
+             subscriptionid: 1234,
+             secrets: {                  // Individual Secrets from SecretStore
+               'my_secret_1': {
+                  source: secretStoreAz.id
+                  key: 'secret.one'
+                }
+               'my_secret_2': {
+                  source: secretStoreAzPayment.id
+                  key: 'secret.two'
+               }
+             }
+          },
+          {
+             subscriptionid: 1234,
+             tenant_id: '745fg88bf-86f1-41af-'
+             alias: 'az-paymentservice', 
+          }]
+          'gcp': [
+            {
+              project: 1234,
+              regions: ['us-east1', 'us-west1']
+            }
+          ]
+          'oraclepass': [
+            {
+              database_endpoint: "...",
+              java_endpoint: "...",
+              mysql_endpoint: "..."
+            }
+          ]
+        }
+     }
+     env: {
+        'MY_ENV_VAR_1': 'my_value'
+        secrets: {                       // Individual Secrets from SecretStore
+        'MY_ENV_VAR_2': {
+            source: secretStoreConfig.id
+            key: 'envsecret.one'
+          }
+        'MY_ENV_VAR_3': {
+            source: secretStoreConfig.id
+            key: 'envsecret.two'
+          }
+        }
+      }   
+    }
  }
  recipes: {      
    ...
  }
}


```

**Option 3** - Environment Variables : With this option a question was raised if 'env' could be updated to 'envVariables'. We decided against it for consistency (with environment variables used in Applications.Core/Containers resource) and decided to keep the name as env as described in Option 2 above.

``` diff
resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'dsrp-resources-env-recipes-context-env'
  location: 'global'
  properties: {
  ...
  recipeConfig: {
      ... 
      terraform: {
         ...
         providers: [...]         // Same as Option 2
+     envVariables: {                     // ** Change from Option 2 **
+       'MY_ENV_VAR_1': 'my_value'
+        secrets: {                 // Individual Secrets from SecretStore
+           'MY_ENV_VAR_2': {
+              source: secretStoreConfig.id
+              key: 'envsecret.one'
+            }
+           'MY_ENV_VAR_3': {
+              source: secretStoreConfig.id
+              key: 'envsecret.two'
+           }
+        }
+      }   
+    }
  }
  recipes: {      
    ...
  }
}
```
**Option 4** - Environment Variables: With the structure described in Option 2 for environment variables, we are unable to use a extends Record\<string\> as  described in the API design section:

```
model EnvironmentVariables extends Record<string>{
  secrets?: Record<ProviderSecret>
}
```
and in order to continue to maintain type strictness for EnvironmentVariables, we discussed the following design amongst others and are going ahead with:

``` diff
resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'dsrp-resources-env-recipes-context-env'
  location: 'global'
  properties: {
  ...
    recipeConfig: {
      ... 
      terraform: {
         ...
         providers: [...]         // Same as Option 2
+     env: {                
+         'MY_ENV_VAR_1': 'my_value'
+     } 
+     envSecrets: {      // Individual Secrets from SecretStore
+         'MY_ENV_VAR_2': {
+            source: secretStoreConfig.id
+            key: 'envsecret.one'
+         }
+        'MY_ENV_VAR_3': {
+            source: secretStoreConfig.id
+            key: 'envsecret.two'
+         }
+     }
+  }   
+}
  recipes: {      
    ...
  }
}

```
Limitations: Customers may store sensitive data in other formats which may not be supported. eg. sensitive data is stored on files, which customers will not currently be able to load on disk in applications-rp where tf init/apply commands are run.
Containerization work may alleviate this limitations. Further design work will be needed for towards this which is planned for the near future. 

### API design

***Model changes providers***

### Option 1:
```
Addition of new property to TerraformConfigProperties in `recipeConfig` under environment properties.

model TerraformConfigProperties{
  @doc(Specifies authentication information needed to use private terraform module repositories.)  
  authentication?: AuthConfig
+ providers?: Array<ProviderConfig>
}

@doc("ProviderConfig specifies provider configurations needed for recipes")
model ProviderConfig {
 name: string
 properties: ProviderConfigProperties
}

@doc("ProviderConfigProperties specifies provider configuration details needed for recipes")
model ProviderConfigProperties extends Record<unknown> {
  @doc("The secrets for referenced resource")
  secrets?: Record<ProviderSecret>;
}
```
### Option 2:
```
Addition of new property to TerraformConfigProperties in `recipeConfig` under environment properties.

model TerraformConfigProperties{
  @doc(Specifies authentication information needed to use private terraform module repositories.)  
  authentication?: AuthConfig
  providers?: Record<Array<ProviderConfigProperties>>
}

@doc("ProviderConfigProperties specifies provider configuration details needed for recipes")
model ProviderConfigProperties extends Record<unknown> {
  @doc("The secrets for referenced resource")
  secrets?: Record<ProviderSecret>;
}
```
***Model changes env***
```
Addition of new property to RecipeConfigProperties under environment properties.

@doc("Specifies recipe configurations needed for the recipes.")
model RecipeConfigProperties {
  @doc("Specifies the terraform config properties")
  terraform?: TerraformConfigProperties;
+ env?: EnvironmentVariables
}

@doc("EnvironmentVariables describes structure enabling environment variables to be set")
model EnvironmentVariables extends Record<string>{
  secrets?: Record<ProviderSecret>
}

@doc("Specifies the secret details")
model ProviderSecret {
  @doc("The resource id for the secret store containing credentials")
  source: string;
  key: string;
}

```

### Option 3:
Providers section is same as Option 2

***Model changes env***
```
Addition of new property to RecipeConfigProperties under environment properties.

@doc("Specifies recipe configurations needed for the recipes.")
model RecipeConfigProperties {
  @doc("Specifies the terraform config properties")
  terraform?: TerraformConfigProperties;
+ envVariables?: EnvironmentVariables
}

@doc("EnvironmentVariables describes structure enabling environment variables to be set")
model EnvironmentVariables extends Record<unknown>{
  secrets?: Record<RecipeSecret>
}

@doc("Specifies the secret details")
model RecipeSecret {
  @doc("The resource id for the secret store containing credentials")
  source: string;
  key: string;
}

```

### Option 4: Final Design

``` 
Addition of new property to TerraformConfigProperties in `recipeConfig` under environment properties.

model TerraformConfigProperties{
  @doc(Specifies authentication information needed to use private terraform module repositories.)  
  authentication?: AuthConfig
  providers?: Record<Array<ProviderConfigProperties>>
}

@doc("ProviderConfigProperties specifies provider configuration details needed for recipes")
model ProviderConfigProperties extends Record<unknown> {
  @doc("The secrets for referenced resource")
  secrets?: Record<SecretReference>;
}
```

***Model changes env***
``` 
Addition of new property to RecipeConfigProperties under environment properties.

@doc("Specifies recipe configurations needed for the recipes.")
model RecipeConfigProperties {
  @doc("Specifies the terraform config properties")
  terraform?: TerraformConfigProperties;
+ env?: EnvironmentVariables
+ envSecrets?: Record<SecretReference>
}

@doc("EnvironmentVariables describes structure enabling environment variables to be set")
model EnvironmentVariables extends Record<string>{}

@doc("Specifies the secret details")
model SecretReference {
  @doc("The resource id for the secret store containing credentials")
  source: string;
  key: string;
}

```
## Decision on Options above:
We initially decided to go ahead with Option 2 and discussed adding validation to check number of provider configurations to be a minimum of 1. Option 2 helps users keep track of all provider configurations for a provider in one place and lowers probability of, say, duplication of provider configurations if it is laid out in one list as in Option 1. Also, we can enforce some constraints on, say, minimum number of configurations for a provider. Option 2 is optimized for multiple provider configurations per provider and that may not apply for every provider configuration that users set up.

Notes: After initial work on implementation we encountered some questions and issues with EnvironmentVariables (listed in Options 3 and 4) above which we brought up in our design meetings and have updated the EnvironmentVariables API design and renamed ProviderSecret to SecretReference.
The latest API design is described in Options 4. 



## Alternatives considered

Mentioned under Limitations above, work to containerize running terraform jobs was considered as a precursor to this work. The time sensitivity for unblocking customers on ability to configure and use providers they use today was given a priority and current design held. Containerization will be taken up as a parallel effort in the near future.

## Test plan (In Progress)

#### Unit Tests
- Update  conversion unit tests to validate providers, env type and later secrets under recipeConfig.
- Update  controller unit tests for providers, env, secrets.
- Unit tests for functions related to building provider config with new provider data when module is loaded.
- Unit tests for secret retrieval and handling/building provider config.



#### Functional tests
- Add e2e test to verify multiple provider configuration is created as expected.
- (TBD) Discuss approach to validation of provider configuration: Is it possible to be static data and not a actual provider? Do we use provider like random?

## Security

Largely following security considerations and secret handling described in design for private terraform modules: [Design document for Private Terraform Repository](https://github.com/radius-project/design-notes/blob/3644b754152edc97e641f537b20cf3d87a386c43/recipe/2024-01-support-private-terraform-repository.md)
The work done here will be to read, decipher values and secrets and environment variables set by user in the *providers* block, to build internal terraform configuration file used by tf setenv, init and apply commands.

## Monitoring

## Development plan
We're breaking down the effort into smaller units in order to enable parallel work and faster delivery of this feature within a sprint.
The user stories created are as follows:
The numbers indicate sequential order of work that can be done. Having said that, work for numbers 1 and 2 are going ahead in parallel and we will resolve conflicts as PRs get merged.
1. Update Provider DataModel, TypeSpec, Convertor, json examples
1. Update Environment Variables DataModel, TypeSpec, Convertor, json examples
1. Documentation
2. Build Provider Config (minus secrets)
2. Process, update environment variables - minus secrets
3. Functional Tests
4. Update Secret DataModel, TypeSpec, Convertor, json examples
4. Secret processing - Providers + Environment Variables

The objective is to deliver within a sprint. We will create unit tests for each task mentioned above as it is completed and test functionality.
It will then be possible to deliver the feature in the following order:
1. Building provider configuration based on provider configuration block
2. Updating environment variables
3. Updating provider configuration and environment variables with processing secrets.

We've currently decided on Secret processing to be taken up following completion of building providing provider based on providers block and env variable block. The goal is to deliver as much functionality as possible within this sprint.

## High Level Design details:
Disclaimer: The following are early assessments and may change as we progress:
No changes are anticipated to payload in Driver and Engine.
The new types containing provider and environment variable data are contained within environment configuration which will be now passed into constructor of TerraformConfig as part of Private Terraform Module work. [link](https://github.com/radius-project/design-notes/blob/3644b754152edc97e641f537b20cf3d87a386c43/recipe/2024-01-support-private-terraform-repository.md) 
Large amount of work will be within pkg/terraform/config where we process new providers and env blocks and update current processing for building provider configuration.

## Open issues/questions
Do we consider first class support for GCP and other popular providers or continue with a generic approach??
Answer-We should start with a generic approach to unlock everything, and then add first-class support where users request it later on. That we support everything, and then we can add convenience later on.
