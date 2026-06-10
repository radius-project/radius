# Topic: Direct Recipe Modules

* **Author**: Reshma Abdul Rahim (@Reshrahim)

## Topic Summary

Radius Recipes enable platform engineers to define reusable infrastructure templates that application developers can consume without understanding the underlying cloud resources. Today, to use an existing community module e.g., an AWS RDS Terraform module from the Terraform Registry, or an Azure Verified Bicep Module as a Recipe, a platform engineer must write a thin recipe wrapper that adds Radius-specific conventions — namely a `context` input variable and a structured `result` output — publish it to a distribution source like a container registry or Git, and then it's ready for use. This extra layer adds maintenance burden, version drift risk, and a barrier to adoption.

**Direct Recipe Module Support removes this requirement.** Platform engineers can now point a Recipe's `location` directly at any standard Bicep or Terraform module — no wrapper needed. Radius automatically resolves input values into the module's parameters at deploy time and maps the module's outputs back to the Resource Type's properties. The result is a simpler workflow: find a module, reference it, configure the parameter and output mappings, and deploy.

### Top level goals

- Enable platform engineers to leverage community owned Terraform registry modules and Azure Verified Modules directly as Radius Recipes
- Eliminate the need to write a Recipe wrapper, publish, and maintain these Recipes separately from the underlying module
- Scale the `resource-types-contrib` repository with tested Recipes by referencing community modules directly, eliminating wrapper maintenance overhead
  
### Non-goals (out of scope)

1. Local filesystem paths as `location` sources. Modules must be fetched from remote registries or Git; local paths are not supported.
2. Automatic module version bumping. Platform engineers explicitly pin module versions in their Recipe Packs. Auto-upgrading could introduce breaking changes silently.
3. Custom retry logic for module fetch failures. Fetch retries are delegated to the underlying IaC engine (Terraform CLI / Bicep). Radius does not add its own retry layer.
4. New observability infrastructure. This feature uses existing Radius logging, tracing, and metrics. No new telemetry systems are introduced.

## User profile and challenges

### User persona(s)

**Platform Engineer**: Responsible for defining infrastructure Recipes that application developers consume. Manages Recipe Packs, environments, and provider configurations. Typically works across multiple teams and maintains a catalog of infrastructure patterns.

**Application Developer**: Consumes Recipes by declaring resources with properties (e.g., `database: 'mydb'`, `size: 's'`). Does not need to understand the underlying IaC module or infrastructure details.

### Challenge(s) faced by the user

1. **Adoption friction** — Every module requires a custom Recipe wrapper that must be published and versioned separately before it can be used. Using a community Terraform module requires writing a Recipe wrapper, publishing the wrapper to a distribution source and then adding it to Recipe Packs for consumption.

2. **Maintenance burden** — Upstream module updates require wrapper changes, validation, and republishing, creating version drift over time. Platform engineers must track upstream releases and update wrappers accordingly.

3. **Ecosystem lockout** — Thousands of production-ready community modules (Terraform registry, Azure Verified Modules) cannot be used directly, limiting the value proposition of Radius Recipes.

### Positive user outcome

Platform engineers can easily configure a Recipe using any existing module setting `location` directly — zero wrapping, zero republishing. The entire Terraform and Bicep module ecosystem becomes immediately usable as Radius Recipes.

## Key scenarios

### Scenario 1: Terraform Registry Module

A platform engineer sets `location` to a Terraform registry URI (e.g., `registry.terraform.io/terraform-aws-modules/rds/aws:5.9.0`) and the system deploys it by automatically resolving developer set properties via `context` as Terraform input variables, and mapping module outputs to Resource Type properties via the `outputs` field.

### Scenario 2: Azure Verified Modules (OCI)

A platform engineer sets `location` to an Azure Verified Module OCI reference (e.g., `br:mcr.microsoft.com/bicep/avm/res/storage/storage-account:0.14.3`) and the system deploys it via ARM, passing resolved parameters and mapping outputs to Resource Type properties via the `outputs` field.

### Scenario 3: Private Git-hosted Terraform Module

A platform engineer references an internal module hosted in a private Git repository (e.g., `git::https://github.com/contoso/infra-modules.git//modules/vpc?ref=v2.0`). This supports organizations that maintain proprietary modules outside the public Terraform Registry. The system clones the module at the specified ref and executes it using the same parameter resolution and output mapping as registry modules.

## Key dependencies and risks

- **Community module coverage** — Not every Radius Resource Type will have a suitable community module available. Platform engineers may still need to author custom modules for Resource Types without community coverage or where community modules don't align with organizational requirements.
- **Terraform version compatibility** — Direct modules may use features from newer Terraform versions. Mitigation: Terraform settings feature enables specifying the Terraform version.
- **Module API stability** — Direct modules expose platform engineers to upstream breaking changes. Mitigation: version pinning via `?ref=` for Git or registry version constraints.
- **Expression resolution correctness** — Unresolved `{{context.*}}` expressions are left as literal strings, which may cause cryptic downstream errors from the IaC engine. Mitigation: clear documentation of available expression paths.

## Current state

Today, Radius only supports Recipes that are purpose-built for Radius. Both Terraform and Bicep modules must include a `context` input variable (carrying Radius runtime metadata) and a structured `result` output (returning resource values back to Radius). Any community or third-party module that lacks these conventions cannot be used as a Recipe without first wrapping it.

Recipe Packs (`Radius.Core/recipePacks`) already enable platform engineers to group Recipe definitions by Resource Type. This direct module support extends that foundation by adding an `outputs` mapping to `RecipeDefinition` and broadening `location` to accept standard module sources that do not follow Radius wrapper conventions.

## Details of user problem

When I want to use a community Terraform module (like `terraform-aws-modules/rds/aws`) as a Radius Recipe, I have to write a wrapper module that adds the `context` variable and `result` output, publish my wrapper to a Git repository or OCI registry based on the IaC engine, and then reference my wrapper in the Recipe definition. If the upstream module releases a new version, I have to update my wrapper to pin the new version, validate that the wrapper still passes through the correct inputs/outputs, and republish it — even if the module's interface hasn't changed. This creates a maintenance burden that scales with the number of modules in my catalog.

## Desired user experience outcome

After this feature, I can set `location` directly to `registry.terraform.io/terraform-aws-modules/rds/aws:5.9.0` on my Recipe definition, configure `parameters` with `{{context.*}}` expressions for dynamic values, and set `outputs` to map the module's outputs to my Resource Type's properties. The module doesn't need to know about Radius. When my application developer deploys a `Radius.Data/mySqlDatabases` resource, the system resolves expressions, passes parameters to the module, executes it, and maps outputs — all without any wrapper.

### Detailed user experience

1. Platform engineer creates a RecipePack pointing directly at an upstream module:

   ```bicep
   resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
     name: 'data-pack'
     properties: {
       recipes: {
         'Radius.Data/mySqlDatabases': {
           kind: 'terraform'
           location: 'registry.terraform.io/terraform-aws-modules/rds/aws:5.9.0'
           // Specify the module's input parameters
           parameters: {
             identifier: '{{context.resource.name}}'
             db_name: '{{context.resource.properties.database}}'
             engine: 'mysql'
             instance_class: 'db.t3.micro'
           }
           // Map the module's outputs to the resource's properties
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

   resource env 'Radius.Core/environments@2025-08-01-preview' = {
     name: 'aws-dev'
     properties: {
       recipePacks: [recipepack.id]
       recipeParameters: {
         'Radius.Data/mySqlDatabases': {
           backup_retention_days: 2
           geo_redundant_backup_enabled: false
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
5. Application reads `resource.properties.host`, `resource.properties.port` etc as the resolved values from the module outputs.

## Key investments

### Feature 1: Direct Module Execution

Use any standard Bicep or Terraform module directly as a Recipe by pointing `location` at the module source. The system automatically detects that the module is not a Radius wrapper (no `context` variable), downloads it, and executes it through the existing driver — no wrapper needed.

### Feature 2: Template Expression Resolution

A `{{context.*}}` expression system that resolves Radius application runtime context values into Recipe parameters at deploy time. Supports resource metadata, application/environment info, Kubernetes runtime, Azure, and AWS provider context. Includes single-level ternary expressions for conditional value mapping.

### Feature 3: Output Mapping

An `outputs` field on `RecipeDefinition` that maps module output names to resource property names. Provides a stable property interface for resource consumers regardless of the underlying module's output naming. Sensitive outputs are automatically routed to the Secrets map.

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
      'Radius.Data/mySqlDatabases': {
        backup_retention_days: 2
        geo_redundant_backup_enabled: false
      }
    }
  }
}
```

Environment parameters merge with Recipe-level parameters. Environment takes precedence for overlapping keys.

### Secrets Output Mapping

Properties marked as `x-radius-sensitive` in the Resource Type definition are automatically handled by Radius — their values are encrypted at rest and not exposed in plain text through the API. When a module output is mapped to a property that has `x-radius-sensitive: true`, Radius encrypts the value before storing it:

```bicep
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'aws-data-pack'
  properties: {
    recipes: {
      'Radius.Data/mySqlDatabases': {
        kind: 'terraform'
        location: 'registry.terraform.io/terraform-aws-modules/rds/aws:5.9.0'
        parameters: {
          identifier: '{{context.resource.name}}'
          db_name: '{{context.resource.properties.database}}'
          engine: 'mysql'
          instance_class: 'db.t3.micro'
          manage_master_user_password: true
        }
        outputs: {
          host: 'db_instance_address'
          port: 'db_instance_port'
          database: 'db_instance_name'
          password: 'db_master_user_secret_arn'       // mapped to x-radius-sensitive property → encrypted by Radius
        }
      }
    }
  }
}
```

The platform engineer does not need to separate values and secrets in the `outputs` mapping. Radius determines sensitivity from the Resource Type schema — any output mapped to a property with `x-radius-sensitive: true` is automatically encrypted.
