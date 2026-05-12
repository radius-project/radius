# Feature: Direct Module Support via Recipe Packs

Enable platform engineers to use existing Terraform or Azure verified modules directly as recipes within Radius recipe packs — without wrapping, republishing, or creating Radius-specific recipe artifacts.

## Problem

Today, to use an existing IaC module with Radius, engineers must wrap the module into a Radius recipe format, publish it to a registry, and then reference it. This is redundant work. Recipe packs should allow pointing directly at an existing Terraform or AVM module and have it work as-is.

## Recipe Packs as Environment Configuration

Recipe packs are resources that define a collection of recipes for different resource types. Environments reference recipe packs via the `recipePacks` property. A recipe pack bundles:

- **Recipe definitions** per resource type (e.g., `Radius.Data/postgreSqlDatabases`)
- **Recipe kind** (`terraform` or `bicep`)
- **Recipe location** (module source — Terraform registry path, Git URL, etc.)
- **Recipe parameters** with support for `{{context.*}}` template expressions
- **Output mappings** (`outputs`) that map resource property names to module output names

## Input and Output Mapping

The key design challenge is matching a module's input variables to resource properties and the module's outputs to computed properties in Radius:

- **Inputs**: Recipe parameters are defined on the recipe pack with `recipeParameters`. Template expressions like `{{context.resource.name}}` allow injecting Radius context at deploy time. Environment-level `recipeParameters` can provide additional defaults per resource type.
- **Outputs**: The `outputs` field on the recipe definition maps resource schema property names to module output names (e.g., `host: 'hostname'` means the resource's `host` property comes from the module's `hostname` output). For direct modules, all outputs are passed through to resource properties, with `outputs` acting as an optional rename/filter layer.

## Example Bicep

```bicep
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'pg-recipepack'
  properties: {
    recipes: {
      'Radius.Data/postgreSqlDatabases': {
        recipeKind: 'terraform'
        recipeLocation: 'ballj/postgresql/kubernetes'
        recipeParameters: {
          namespace: 'default'
          name: '{{context.resource.name}}'
          object_prefix: 'myapp-pg'
          image_tag: 'latest'
        }
        outputs: {
          host: 'hostname'
          port: 'port'
          database: 'database_name'
          username: 'username'
          secretName: 'password_secret'
        }
      }
    }
  }
}

resource myenv 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'my-env'
  properties: {
    providers: {
      kubernetes: { namespace: 'default' }
    }
    recipePacks: [
      recipepack.id
    ]
  }
}
```
