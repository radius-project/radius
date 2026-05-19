# Topic: Direct Recipe Modules

* **Author**: Reshma Abdul Rahim (@Reshrahim)

## Topic Summary

Direct Recipe Module Support enables platform engineers to use any standard Bicep or Terraform module as a Radius Recipe without writing a Radius-specific wrapper. Today, using a module as a Radius Recipe requires a wrapper that conforms to Radius conventions (a `context` input variable, a structured `result` output). This feature eliminates the wrapper: point `location` directly at a standard module, and the system handles input resolution (injecting Radius context) and output resolution (mapping module outputs to resource properties) externally.

### Top level goals

- Enable platform engineers to leverage community owned Terraform registry modules and Azure Verified Modules directly as Radius Recipes
- Eliminate the need to write a Recipe wrapper, publish, and maintain these Recipes separately from the underlying module
- Eliminate the need for Radius to maintain a catalog of wrapped Recipes in the `resource-types-contrib` repository thus reducing maintenance overhead and surface area for supply chain attacks
  
### Non-goals (out of scope)

- Local filesystem paths as `location` sources
- Chained/nested ternary expressions (V1 supports single-level only)
- Automatic module version bumping
- Custom retry logic for module fetch failures (delegated to underlying IaC engine)
- New observability infrastructure (uses existing logging, tracing, metrics)

## User profile and challenges

### User persona(s)

**Platform Engineer**: Responsible for defining infrastructure recipes that application developers consume. Manages recipe packs, environments, and provider configurations. Typically works across multiple teams and maintains a catalog of infrastructure patterns.

**Application Developer**: Consumes recipes by declaring resources with properties (e.g., `database: 'mydb'`, `size: 's'`). Does not need to understand the underlying IaC module or infrastructure details.

### Challenge(s) faced by the user

1. **Adoption friction** — Every module requires a custom recipe wrapper that must be published and versioned separately before it can be used. Using a community Terraform module requires writing a Recipe wrapper, publishing the wrapper to a distribution source and then adding it to Recipe packs for consumption.

2. **Maintenance burden** — Upstream module updates require wrapper changes, validation, and republishing, creating version drift over time. Platform engineers must track upstream releases and update wrappers accordingly.

3. **Ecosystem lockout** — Thousands of production-ready community modules (Terraform registry, Azure Verified Modules) cannot be used directly, limiting the value proposition of Radius recipes.

### Positive user outcome

Platform engineers can easily configure a recipe using any existing module setting `location` directly — zero wrapping, zero republishing. The entire Terraform and Bicep module ecosystem becomes immediately usable as Radius recipes.

## Key scenarios

### Scenario 1: Terraform Registry Module

A platform engineer sets `location` to a Terraform registry path (e.g., `terraform-aws-modules/rds/aws`) and the system deploys it by automatically resolving developer set properties via `context` as Terraform input variables, and mapping module outputs to resource properties via the `outputs` field.

### Scenario 2: Azure Verified Modules (OCI)

A platform engineer sets `location` to an Azure Verified Module OCI reference (e.g., `br:mcr.microsoft.com/bicep/avm/res/storage/storage-account:0.14.3`) and the system deploys it via ARM, passing resolved parameters and mapping outputs to resource properties via the `outputs` field.

### Scenario 3: Git-hosted Terraform Module

A platform engineer references a private or public Git-hosted module (`git::https://github.com/org/module.git//subdir?ref=v2.0`) and the system clones and executes it.

## Key dependencies and risks

- **Community module coverage** — Not every Radius resource type will have a suitable community module available. Platform engineers may still need to author custom modules for resource types without community coverage or where community modules don't align with organizational requirements.
- **Terraform version compatibility** — Direct modules may use features from newer Terraform versions. Mitigation: Terraform settings feature enables specifying the Terraform version.
- **Module API stability** — Direct modules expose platform engineers to upstream breaking changes. Mitigation: version pinning via `?ref=` for Git or registry version constraints.
- **Expression resolution correctness** — Unresolved `{{context.*}}` expressions are left as literal strings, which may cause cryptic downstream errors from the IaC engine. Mitigation: clear documentation of available expression paths.

## Current state

Radius currently supports recipes through wrapped modules that conform to specific conventions:
- **Terraform**: Module must declare a `context` input variable and produce a structured `result` output
- **Bicep**: Module must accept a `context` parameter and produce a `result` output

Recipe Packs (`Radius.Core/recipePacks`) were introduced to group recipe definitions by resource type. This feature builds on Recipe Packs by extending `RecipeDefinition` with `outputs` mapping and broadening `location` to accept standard module sources.

## Details of user problem

When I want to use a community Terraform module (like `terraform-aws-modules/rds/aws`) as a Radius recipe, I have to write a wrapper module that adds the `context` variable and `result` output, publish my wrapper to a Git repository or OCI registry based on the IaC engine, and then reference my wrapper in the recipe definition. If the upstream module releases a new version, I have to update my wrapper, test it, and republish. This creates a maintenance burden that scales with the number of modules in my catalog.

## Desired user experience outcome

After this feature, I can set `location` directly to `terraform-aws-modules/rds/aws` on my recipe definition, configure `parameters` with `{{context.*}}` expressions for dynamic values, and set `outputs` to map the module's outputs to my resource type's properties. The module doesn't need to know about Radius. When my application developer deploys a `Radius.Data/mySqlDatabases` resource, the system resolves expressions, passes parameters to the module, executes it, and maps outputs — all without any wrapper.

### Detailed user experience

1. Platform engineer creates a RecipePack pointing directly at an upstream module:

   ```bicep
   resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
     name: 'data-pack'
     properties: {
       recipes: {
         'Radius.Data/mySqlDatabases': {
           kind: 'terraform'
           location: 'terraform-aws-modules/rds/aws'
           parameters: {
             identifier: '{{context.resource.name}}'
             db_name: '{{context.resource.properties.database}}'
             engine: 'mysql'
             instance_class: 'db.t3.micro'
           }
           outputs: {
             host: 'db_instance_address'
             port: 'db_instance_port'
             database: 'db_instance_name'
           }
         }
       }
     }
   }
   ```

2. Application developer deploys a resource with properties:

   ```bicep
   resource mysql 'Radius.Data/mySqlDatabases@2025-08-01-preview' = {
     name: 'orders-db'
     properties: {
       environment: environment
       application: app.id
       database: 'ordersdb'
     }
   }
   ```

3. System resolves expressions (`{{context.resource.name}}` → `'orders-db'`, `{{context.resource.properties.database}}` → `'ordersdb'`), merges parameters, and executes Terraform against the upstream module
4. Module outputs are mapped to resource properties via the `outputs` definition
5. Application developer reads `resource.properties.host`, `resource.properties.port` etc.

## Key investments

### Feature 1: Direct Module Execution

Use any standard Bicep or Terraform module directly as a recipe by pointing `location` at the module source. The system automatically detects that the module is not a Radius wrapper (no `context` variable), downloads it, and executes it through the existing driver — no wrapper needed.

### Feature 2: Template Expression Resolution

A `{{context.*}}` expression system that resolves Radius application runtime context values into recipe parameters at deploy time. Supports resource metadata, application/environment info, Kubernetes runtime, Azure, and AWS provider context. Includes single-level ternary expressions for conditional value mapping.

### Feature 3: Output Mapping

An `outputs` field on `RecipeDefinition` that maps module output names to resource property names. Provides a stable property interface for resource consumers regardless of the underlying module's output naming. Sensitive outputs are automatically routed to the Secrets map.

---

## Usage Examples

### Terraform Registry Module (AWS RDS)

```bicep
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'aws-data-pack'
  properties: {
    recipes: {
      'Radius.Data/mySqlDatabases': {
        kind: 'terraform'
        location: 'terraform-aws-modules/rds/aws'
        parameters: {
          identifier: '{{context.resource.name}}'
          db_name: '{{context.resource.properties.database}}'
          engine: 'mysql'
          engine_version: '8.0'
          instance_class: 'db.t3.micro'
          manage_master_user_password: true
          create_db_subnet_group: true
          subnet_ids: subnetIds
          vpc_security_group_ids: vpcSecurityGroupIds
        }
        outputs: {
          host: 'db_instance_address'
          port: 'db_instance_port'
          database: 'db_instance_name'
          secretName: 'db_instance_master_user_secret_arn'
        }
      }
    }
  }
}
```

### Azure Verified Module (PostgreSQL) with T-Shirt Sizing

```bicep
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'azure-postgres-pack'
  properties: {
    recipes: {
      'Radius.Data/postgreSqlDatabases': {
        kind: 'terraform'
        location: 'Azure/avm-res-dbforpostgresql-flexibleserver/azurerm'
        parameters: {
          name: 'pg-{{context.resource.name}}'
          location: 'eastus2'
          sku_name: '{{context.resource.properties.size == "s" ? "B_Standard_B1ms" : "GP_Standard_D2s_v3"}}'
          storage_mb: '{{context.resource.properties.size == "s" ? "32768" : "65536"}}'
          tags: {
            environment: '{{context.environment.name}}'
            application: '{{context.application.name}}'
          }
        }
        outputs: {
          host: 'fqdn'
          port: 'port'
          database: 'database_name'
          username: 'administrator_login'
        }
      }
    }
  }
}
```

### Environment-Level Parameter Overrides

```bicep
resource devenv 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'dev'
  properties: {
    recipePacks: [recipepack.id]
    recipeParameters: {
      backup_retention_days: '2'
      geo_redundant_backup_enabled: 'false'
    }
  }
}
```

Environment parameters merge with recipe-level parameters. Environment takes precedence for overlapping keys.
