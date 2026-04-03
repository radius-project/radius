# Validation for Terraform Recipe Template Paths

* **Author**: Shruthi Kumar (@sk593)

## Overview

Currently, we support only Terraform registry and HTTP URLs as allowed module sources for Terraform recipe template paths. However, we lack a proper validation mechanism for when an unsupported source is given during either recipe registration or recipe deployment. Consequently, when a deployment fails, the reason provided to users is ambiguous resulting into a poor user experience.

## Terms and definitions

| Term     | Definition                                                                                                                                                                                                 |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Terraform module source | The source argument in a module block tells Terraform where to find the source code for the desired child module |
| Module registry | A module registry is the native way of distributing Terraform modules for use across multiple configurations |
| Terraform registry | Terraform Registry is an index of modules shared publicly |
| HTTP URLs | When you use an HTTP or HTTPS URL, Terraform will make a GET request to the given URL, which can return another source address | 

## Objectives

> **Issue Reference:** https://github.com/radius-project/radius/issues/6642

### Goals

- If a supported module source for a Terraform recipe is provided, the recipe registration should succeed. 
- If an unsupported module source for a Terraform recipe is provided, the recipe registration should fail and return a detailed error message to the user.


### Non goals

- Adding support and testing for additional module sources (Github, S3, etc). These will be addressed as needed based on customer feedback.
- Adding support for private module sources.   

### User scenarios (optional)

#### User story 1

As a Radius user, I want to define a Terraform recipe with a Terraform module template path. My module source is a Terraform registry or HTTP URL. I am able to register my Terraform recipe with this source and deploy it successfully. 

#### User story 2

As a Radius user, I want to define a Terraform recipe with a Terraform module template path. My module source is not the supported Terraform registry or HTTP URLs (i.e. Github, local paths, S3 buckets, etc). When I try to register the recipe to my environment, I receive an error message notifying me that I can only provide a Terraform module registry or HTTP URL as a valid module source using Radius. 

## Design

### Design details

### API design (if applicable)
N/A. We are updating an existing API to include more validation so this will potentially cause breaking changes if users try to run Terraform with an unsupported module source. However, we expect this to be the case as we're only allowing specific module sources. 


#### Validate module sources as part of the CreateOrUpdateEnvironment API endpoint 
Right now, we only return an error at deployment time. We'd like to return an error while the recipe is being registered to the environment. To do so, we'll add validation as part of the `CreateOrUpdateEnvironment` API. This requires no new API changes, just an update to how we handle environment conversion.

We already have a check for local path module sources [here](https://github.com/radius-project/radius/blob/40c91fdc3a4dd3ac04906094dc8302f7232d700d/pkg/corerp/api/v20231001preview/environment_conversion.go#L303). Instead of just rejecting local module source paths, we'll only accept Terraform registries and HTTP URLs and reject everything else.  

#### Validation steps
Terraform registries: 
Terraform has code to check whether a module source is a Terraform recipe as part of the `terraform-registry-address` [package](https://github.com/hashicorp/terraform-registry-address/blob/main/module.go#L44). We will use this package to validate Terraform module registries. 

HTTP URLs: 
Terraform treats HTTP URLs as remote addresses and doesn't have functionality for checking HTTP URLs in isolation. We will need to implement our own validation for HTTP URLs. Go provides validation for URIs as part of the `url` [package](https://pkg.go.dev/net/url#ParseRequestURI), but we will likely need additional checks to ensure that a `.zip`/`.tar`/etc suffix is present as well. 

Any module source that is not a Terraform registry or an HTTP URL will be treated as unsupported. We will return a `NewClientErrInvalidRequest` [error](https://github.com/radius-project/radius/blob/main/pkg/armrpc/api/v1/error.go#L64) in this case. The error message will indicate that we only support Terraform registries and HTTP URLs currently. 

## Alternatives considered

1. Adding a `ValidateTemplateKind` API endpoint (similar to `GetRecipeMetadata`).
This was rejected because the logic could be added to the existing `CreateOrUpdateEnvironment` API endpoint instead. That way, there's less code to add and we can return a validation error before we save any environment information. 
2. Support all Terraform module sources instead of restricting supported sources.
Support for other modules will be done on an as-needed customer basis. This needs to be investigated more to see which module sources need more support and if it's viable to support all of them easily. 


## Test plan

Unit testing:
- Add tests for accepted module sources: HTTP URLs and Terraform module registries 
  - For HTTP URLs, make sure to test negative cases as the Go URL package isn't very robust 
- Add tests for module sources that are not accepted (validate error thrown): local paths, Github, S3, etc 
- Add tests in environment conversion for different module sources (and validate errors/successes)

## Security
Since we're parsing module sources as an input from the user, this has the potential to introduce security issues (i.e. infinite loops, buggy code, etc). We'll be relying on existing libraries instead of writing our own parsers to address this concern. 

## Compatibility (optional)

## Monitoring

## Development plan

- Task 1:  
    - Add logic for validating if module source is an HTTP URL or Terraform module registry and return error otherwise
    - Unit Testing
- Task 2:
    - Update CreateOrUpdateEnvironment API endpoint to include checks for validation (update environment conversion logic specifically)
    - Unit Testing

## Open issues