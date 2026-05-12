# Quickstart: Direct Terraform and AVM Module Support via Recipe Packs

## What This Feature Does

Allows platform engineers to use any standard Terraform module or Azure Verified Modules directly as a recipe's `recipeLocation` in a RecipePack — without wrapping, republishing, or creating Radius-specific artifacts. The system downloads the module at deployment time, passes `recipeParameters` (with `{{context.*}}` expression resolution) as Terraform input variables, and surfaces module outputs as resource properties with optional rename/filter via the `outputs` mapping.

## Usage Examples

### Example 1: Azure Verified Module (AVM) — PostgreSQL Flexible Server with T-Shirt Sizing

Uses the [Azure/avm-res-dbforpostgresql-flexibleserver/azurerm](https://registry.terraform.io/modules/Azure/avm-res-dbforpostgresql-flexibleserver/azurerm) module directly from the Terraform registry. Demonstrates conditional ternary expressions to translate abstract t-shirt sizes (`s`, `m`, `l`) into concrete Azure SKUs and storage configurations.

```bicep
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'azure-postgres-pack'
  properties: {
    recipes: {
      'Radius.Data/postgreSqlDatabases': {
        recipeKind: 'terraform'
        recipeLocation: 'Azure/avm-res-dbforpostgresql-flexibleserver/azurerm'
        recipeParameters: {
          name: 'pg-{{context.resource.name}}'
          location: 'eastus2'
          // Single-level ternary: maps "s" to burstable SKU, else general purpose
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

resource myenv 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'azure-env'
  properties: {
    providers: {
      azure: {
      }
    }
    recipePacks: [
      recipepack.id
    ]
  }
}
```

Application developers simply specify `size: 's'`, `size: 'm'`, or `size: 'l'` on their resource, and the platform engineer's ternary expressions translate these into the appropriate Azure SKU and storage configuration at deploy time.

### Example 2: AWS RDS Module — PostgreSQL Database

Uses the popular [terraform-aws-modules/rds/aws](https://registry.terraform.io/modules/terraform-aws-modules/rds/aws) community module to provision an AWS RDS PostgreSQL instance directly — no wrapping required. Demonstrates ternary expressions for t-shirt size mapping to AWS instance classes and storage.

```bicep
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'aws-data-pack'
  properties: {
    recipes: {
      'Radius.Data/postgreSqlDatabases': {
        recipeKind: 'terraform'
        recipeLocation: 'terraform-aws-modules/rds/aws'
        recipeParameters: {
          identifier: 'pg-{{context.resource.name}}'
          instance_class: '{{context.resource.properties.size == "s" ? "db.t3.micro" : "db.r5.large"}}'
          allocated_storage: '{{context.resource.properties.size == "s" ? "20" : "100"}}'
          db_name: '{{context.resource.name}}'
          create_db_subnet_group: true
          subnet_ids: ['subnet-12345678', 'subnet-87654321']
          vpc_security_group_ids: ['sg-12345678']
          tags: {
            Environment: '{{context.environment.name}}'
            Application: '{{context.application.name}}'
          }
        }
        outputs: {
          host: 'db_instance_address'
          port: 'db_instance_port'
          database: 'db_instance_name'
          username: 'db_instance_username'
        }
      }
    }
  }
}

resource myenv 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'aws-env'
  properties: {
    providers: {
      aws: {
        scope: '/planes/aws/aws/accounts/123456789012/regions/us-east-1'
      }
    }
    recipePacks: [
      recipepack.id
    ]
  }
}
```

## How Outputs Work

The `outputs` mapping on the recipe definition maps resource property names to module output names. Only mapped outputs are surfaced:

```bicep
outputs: {
  host: 'hostname'       // resource.properties.host ← module.hostname
  port: 'port'           // resource.properties.port ← module.port  
  database: 'database_name' // resource.properties.database ← module.database_name
}
```

Sensitive module outputs (marked `sensitive = true` in the module) are stored securely in the `Secrets` map rather than `Values`.

## How Parameters Work

`recipeParameters` map directly to Terraform input variables:

1. **Recipe pack-level parameters** (in `recipeParameters` on recipe definition) → applied to every deployment
2. **Environment-level parameters** (in `recipeParameters` on environment) → merged with recipe pack params, environment takes precedence
3. **No parameter** for optional variables → Terraform uses the module's default value
4. **Missing required variable** → Terraform error surfaced through recipe failure
5. **`{{context.*}}` expressions** → resolved at deploy time against the recipe context

### Environment-Level Parameter Overrides

The environment can override or extend recipe pack parameters. This is useful when the same recipe pack is shared across environments (dev, staging, prod) but certain parameters need to differ.

```bicep
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'pg-pack'
  properties: {
    recipes: {
      'Radius.Data/postgreSqlDatabases': {
        recipeKind: 'terraform'
        recipeLocation: 'Azure/avm-res-dbforpostgresql-flexibleserver/azurerm'
        recipeParameters: {
          name: 'pg-{{context.resource.name}}'
          resource_group_name: '{{context.azure.resourceGroup.name}}'
          postgresql_version: '16'
          sku_name: '{{context.resource.properties.size == "s" ? "B_Standard_B1ms" : "GP_Standard_D2s_v3"}}'
        }
      }
    }
  }
}

resource devenv 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'dev'
  properties: {
    providers: {
      azure: {
        scope: '{{context.azure.resourceGroup.id}}'
      }
    }
    recipePacks: [
      recipepack.id
    ]
    recipeParameters: {
      backup_retention_days: '2'
      geo_redundant_backup_enabled: 'false'
    }
  }
}
```

In this example, the dev environment adds `backup_retention_days` and disables geo-redundant backups. Environment parameters take precedence over recipe pack parameters when both define the same key.

### Template Expression Resolution

Parameter values can contain `{{context.*}}` expressions that inject Radius runtime context into Terraform module variables:

```bicep
recipeParameters: {
  // Direct value — passed as-is to Terraform
  instance_class: 'db.t3.micro'

  // Context expression — resolved at deploy time
  k8s_namespace: '{{context.runtime.kubernetes.namespace}}'

  // Mixed content — expression resolved, literals preserved
  resource_prefix: 'app-{{context.resource.name}}-suffix'

  // Resource properties — access user-specified properties from the resource definition
  db_size: '{{context.resource.properties.size}}'

  // Ternary expression — conditional value mapping at deploy time
  sku_name: '{{context.resource.properties.size == "s" ? "B_Standard_B1ms" : "GP_Standard_D2s_v3"}}'
}
```

### Ternary Expressions

Ternary expressions enable conditional value mapping inside `{{...}}` expressions. They are evaluated at deploy time when `context.*` values are resolved.

**Syntax:** `{{<expr> == "<value>" ? "<trueResult>" : "<falseResult>"}}`

Single-level ternary only (V1 limitation — chained/nested ternaries are not supported):

```bicep
recipeParameters: {
  // Map t-shirt size to Azure PostgreSQL SKU (use separate parameters for multi-way mapping)
  sku_name: '{{context.resource.properties.size == "s" ? "B_Standard_B1ms" : "GP_Standard_D2s_v3"}}'

  // Map size to storage — simple conditional
  storage_mb: '{{context.resource.properties.size == "s" ? "32768" : "65536"}}'
}
```

**Behavior:**
- The left-hand side of `==` is resolved against the context lookup (same paths as regular expressions)
- String comparison is exact-match (case-sensitive)
- The else branch must be a literal string value (not another ternary)
- Values are always returned as strings (Terraform handles type conversion)
- If the condition path is unresolvable, the entire expression is left as-is

> **Note:** For multi-way mapping (e.g., s/m/l → 3 different values), use separate recipe definitions per tier or handle the mapping in the module itself. Chained ternary support is planned for a future release.

### Available Template Expressions

| Expression | Description |
|------------|-------------|
| `{{context.resource.name}}` | Deployed resource name |
| `{{context.resource.id}}` | Deployed resource ID |
| `{{context.resource.type}}` | Deployed resource type |
| `{{context.resource.properties.<key>}}` | User-specified resource property (e.g., `size`, `tier`) |
| `{{context.application.name}}` | Application name |
| `{{context.application.id}}` | Application ID |
| `{{context.environment.name}}` | Environment name |
| `{{context.environment.id}}` | Environment ID |
| `{{context.runtime.kubernetes.namespace}}` | Target Kubernetes namespace |
| `{{context.runtime.kubernetes.environmentNamespace}}` | Environment Kubernetes namespace |
| `{{context.azure.resourceGroup.name}}` | Azure resource group name |
| `{{context.azure.resourceGroup.id}}` | Azure resource group ID |
| `{{context.azure.subscription.subscriptionId}}` | Azure subscription ID |
| `{{context.aws.region}}` | AWS region |
| `{{context.aws.account}}` | AWS account ID |

## Key Behaviors

| Behavior | Details |
|----------|---------|
| **Module download** | Fresh download every deployment (no caching) |
| **Version pinning** | Embedded in `recipeLocation` for registry modules (e.g., `Azure/module/azurerm` uses latest, append version constraint in module config), `?ref=` for Git |
| **Provider config** | Uses existing `recipeConfig.terraform.providers` from environment |
| **State management** | Same Kubernetes secret backend as existing recipes |
| **Error handling** | Terraform errors surfaced directly in recipe failure response |
| **Existing recipes** | Zero behavioral changes — fully backward compatible |
| **`result` output** | If present AND no `outputs` mapping configured, uses wrapped recipe convention |
| **`outputs` mapping** | Takes precedence over `result` when configured — only mapped outputs flow through |

## Development Workflow

### Building

```bash
make build
```

### Running Tests

```bash
# Unit tests for the shared expression resolver
go test ./pkg/recipes/paramresolver/...

# Unit tests for the output mapping utility
go test ./pkg/recipes/outputmapping/...

# Unit tests for the source resolver
go test ./pkg/recipes/source/...

# Unit tests for terraform driver changes
go test ./pkg/recipes/driver/terraform/...

# Unit tests for bicep driver changes
go test ./pkg/recipes/driver/bicep/...

# Unit tests for terraform executor changes
go test ./pkg/recipes/terraform/...

# All recipe package tests
go test ./pkg/recipes/...

# All unit tests
make test
```

### Linting

```bash
make lint
make format-check
```
