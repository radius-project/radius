---
applyTo: '**/*.bicep'
description: Bicep Conventions and Guidelines Instructions
---

# Bicep Conventions and Guidelines Instructions

## Naming Conventions

- When writing Bicep code, use lowerCamelCase for all names (variables, parameters, resources)
- Use resource type descriptive symbolic names (e.g., `storageAccount` not `storageAccountName`)
- Avoid using `name` in a symbolic name as it represents the resource, not the resource's name
- Avoid distinguishing variables and parameters by the use of suffixes
- Name module files descriptively based on their primary resource or purpose (e.g., `storageAccount.bicep`, `appServicePlan.bicep`) instead of generic names like `main.bicep` for shared modules

## Structure and Declaration

- Always declare parameters at the top of files with @description decorators
- Order declarations as: parameters first, then variables, then resources, and finally outputs to keep files predictable and easy to scan
- Use latest stable API versions for all resources
- Use descriptive @description decorators for all parameters
- Specify minimum and maximum character length for naming parameters

## Parameters

- Set default values that are safe for test environments (use low-cost pricing tiers)
- Use @allowed decorator sparingly to avoid blocking valid deployments
- Use parameters for settings that change between deployments

## Variables

- Variables automatically infer type from the resolved value
- Use variables to contain complex expressions instead of embedding them directly in resource properties

## Resource References

- Use symbolic names for resource references instead of reference() or resourceId() functions
- Create resource dependencies through symbolic names (resourceA.id) not explicit dependsOn
- For accessing properties from other resources, use the `existing` keyword instead of passing values through outputs

## Resource Names

- Use template expressions with uniqueString() to create meaningful and unique resource names
- Add prefixes to uniqueString() results since some resources don't allow names starting with numbers

## Child Resources

- Avoid excessive nesting of child resources
- Use parent property or nesting instead of constructing resource names for child resources

## Security

- Never include secrets or keys in outputs
- Use resource properties directly in outputs (e.g., storageAccount.properties.primaryEndpoints)
- Use the @secure() decorator on parameters that hold sensitive values such as passwords, keys, tokens, or connection strings

## Documentation

- Include helpful // comments within your Bicep files to improve readability

---

<!-- End of Bicep Conventions and Guidelines Instructions -->
