# Terraform module versions

* **Author**: `Ryan Nowak (@rynowak)`

## Overview

Recipes enables operators to register external infrastructure-as-code (IaC) templates as part of an environment so they can be used later by developers for infrastructure provisioning. This enables separation of roles between operators (responsible for infrastructure decisions) and developers (responsible for applications and business logic, not infrastructure experts).

As part of the registration for a Terraform recipe, operators provide a module source and version - together these properties determine how the module is loaded by Radius. We expect that the most common kind of source to use for Terraform modules is the official Terraform registry, which implements versioning based on SemVer 2.0 and requires versions for resolving a module.

When designed the version as a required field in the recipe registration, we neglected to consider alternative sources such as HTTP URLs. The current approach cannot support non-registry sources for modules and should be revisited.

## Terms and definitions

| Term     | Definition                                                                                                                                                                                                 |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Source   | Location of a Terraform module. May be a registry, HTTP URL, file path, or several other options.                                                                                                          |
| Registry | A specific protocol and versioned hosting system for Terraform modules. Has an official public registry service as well as private services as part of Terraform cloud. Scant open-source implementations. |

References:
- [Module Sources](https://developer.hashicorp.com/terraform/language/modules/sources#module-sources)
- [Using Modules (in Terraform)](https://developer.hashicorp.com/terraform/language/modules/syntax)

## Objectives

> **Issue Reference:** 

### Goals

- Recipes support the full set of sources that will be widely used by users.
- Recipes registrations provide an intuitive user-experience.
  - Version is required for a registry-based source.
  - Version is NOT ALLOWED for non-registry-based sources.
- We can easily author tests for Terraform Recipes.
  - The OSS implementations of the Terraform registry protocol are not mature or widely used.
  - We discovered this issue using a non-registry host for modules to enable our E2E tests.

### Non goals

- Revisit overall Terraform or recipes versioning approach.
- Rename fields to be consistent with Terraform's terminology. We previously decided on `templatePath` and `templateVersion` to be consistent with our use of the term `template` as a generic term in recipes.
- Credentials support for sources that require them is out of scope for this design, this is focused on version as a required field.
- Creating a "stringified" representation of a source + version similar to an OCI reference.
  - We previously considered this approach and decided it was overly complex.

### User scenarios (optional)

#### Registering a recipe as an operator

As an operator I am responsible for maintaining a set of Terraform recipes for use with Radius. Part of my job is to ensure that we can use these recipes from Radius environments with minimal friction. I need to choose a hosting method for the Terraform modules from the set of [supported options](https://developer.hashicorp.com/terraform/language/modules/sources#module-sources) and use this to store the modules. 

After that I need to configure environments to reference those modules, the configuration that I provide (the source) is different based on the kind of storage I choose. 


## Design


### Design details

For a Terraform recipe we require both the `templatePath` and `templateVersion` today. 

```bicep
resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'contoso-prod'
  properties: {
    recipes: {
      'Applications.Link/extenders': {
        default: {
          templateKind: 'terraform'
          templatePath: 'registry.contoso.com/recipes/s3/my-recipe'
          templateVersion: '1.0.0'
        }
      }
    }
  }
}
```


When we generate a Terraform config to interact with the module, we output something like:

```hcl
module "default" {
  source = "registry.contoso.com/recipes/s3/my-recipe"
  version = "1.0.0"
}
```

This format is used by the Terraform executable to perform whatever operation we need.

----

When `version` is set, Terraform will **assume** the `source` refers to a registry and return a parsing error if it does not.

```hcl
module "testing" {
  source = "http://localhost:8080/test-recipe-1.zip"
  version = "value"
}
```


```txt
❯ terraform get
╷
│ Error: Invalid registry module source address
│ 
│   on main.tf line 2, in module "testing":
│    2:   source = "http://localhost:8080/test-recipe-1.zip"
│ 
│ Failed to parse module registry address: invalid module registry hostname
│ "http:".
│ 
│ Terraform assumed that you intended a module registry source address because you
│ also set the argument "version", which applies only to registry modules.
```

The proposed change is to make `templateVersion` optional. When a `templateVersion` is provided we will set `version` in the generated Terraform config (as shown in the example). When `templateVersion` is omitted we will omit `templateVersion` in the Terraform config.

### API design (if applicable)

`templateVersion` will be made optional.

Using a Bicep example because we don't have CADL yet for environments.

**Before**

```bicep
resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'contoso-prod'
  properties: {
    recipes: {
      'Applications.Link/extenders': {
        default: {
          templateKind: 'terraform'

          // Non-registry-source (URL) doesn't use versions
          templatePath: 'http://recipes.contoso.com/recipes/s3/my-recipe.zip'
          templateVersion: '' // This is required, what should go here?
        }
      }
    }
  }
}
```

**After**

```bicep
resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'contoso-prod'
  properties: {
    recipes: {
      'Applications.Link/extenders': {
        default: {
          templateKind: 'terraform'

          // Non-registry-source (URL)
          templatePath: 'http://recipes.contoso.com/recipes/s3/my-recipe.zip'
        }
      }
    }
  }
}
```

## Alternatives considered

### Template Version remains required

We could consider requiring the template version and dropping support for non-registry sources. That seems like a bad decision because non-registry sources seem important (git, S3, HTTP/HTTPS). Since the data is passed through to Terraform, we don't need to do a lot of work to enable these.

### Template Version is informational an we add a field describing the kind of source

Another approach would be to have users tell us the kind of source. Then we could capture the version for use in reporting and guardrails like stickiness. Since we'd know the kind of source being used, we can generate the right Terraform config.

The downside of this approach is that it couples us to details of Terraform we don't own. It's better for us to be a passthrough to Terraform.

### Template Version is required by supports a 'none' sentinel

If we're worried about users omitting the template version field we could make it required, and instead support a sentinel value like `none` when the source does not support versioning.

This feels obtuse and will likely cause more confusion than it prevents.

## Test plan

Existing unit tests will be updated to reflect the field being optional. Existing unit tests will be updated to cover both the `templateVersion` and no `templateVersion` case for generating the Terraform config.

Functional tests for Terraform recipes will use the HTTP source. We do not need functional tests for other sources as we pass through the source data to Terraform, and we can cover the differences with unit tests.


## Security

No changes to the overall security model. We're relying on the security of Terraform sources and the Terraform tool for the security and integrity of the modules we access.

## Compatibility (optional)

No compatibility impact, we are removing a limitation.

## Monitoring

No changes needed

## Development plan

This is simple enough to be done in a single PR. This will include a functional test (the first functional test for Terraform recipes).

## Open issues

**What's the impact of this on planned stickiness?**

The challenge here is that we previously *assumed* the user would tell us the version of the module they are using. This is required for Bicep and it's required for Terraform registries. However it's NOT required for Terraform sources like S3 or HTTP - they have a weaker concept of versioning.

It's likely that we can provide the strongest guarantees for Bicep and Terraform with registries. 