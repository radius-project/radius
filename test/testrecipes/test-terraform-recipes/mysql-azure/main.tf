terraform {
  required_version = ">= 1.5"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 4.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.6"
    }
  }
}

data "azurerm_resource_group" "rg" {
  name = var.context.azure.resourceGroup.name
}

//////////////////////////////////////////
// Common Radius variables
//////////////////////////////////////////

locals {
  resource_name    = var.context.resource.name
  application_name = var.context.application != null ? var.context.application.name : ""
  environment_name = var.context.environment != null ? var.context.environment.name : ""
}

//////////////////////////////////////////
// Unique server name
//////////////////////////////////////////

# Generates a per-deployment random suffix so re-deploys after a server has
# been deleted (and is in Azure's 7-day soft-delete window) don't collide on
# the globally-reserved server name. The value is stable in Terraform state,
# so re-applies with the same state are idempotent.
resource "random_id" "server_suffix" {
  byte_length = 7
}

//////////////////////////////////////////
// MySQL variables
//////////////////////////////////////////

locals {
  port     = 3306
  database = try(var.context.resource.properties.database, "mysql_db")
  # Azure MySQL Flexible Server accepts only specific version strings.
  # Map common shorthand values to valid versions.
  version = lookup(
    { "8.0" = "8.0.21", "8" = "8.0.21", "5" = "5.7" },
    try(var.context.resource.properties.version, "8.0.21"),
    try(var.context.resource.properties.version, "8.0.21")
  )

  unique_suffix = random_id.server_suffix.hex

  # Azure MySQL server name: lowercase alphanumeric and hyphens, 3-63 chars
  sanitized_server_name = "mysql-${local.unique_suffix}"

  # Database name: alphanumeric and underscores only
  sanitized_database = replace(local.database, "/[^0-9A-Za-z_]/", "_")

  tags = {
    "radapp.io-resource"    = local.resource_name
    "radapp.io-application" = local.application_name
    "radapp.io-environment" = local.environment_name
  }
}

//////////////////////////////////////////
// Credentials
//
// Azure MySQL Flexible Server rejects common admin names ("admin",
// "administrator", "root", etc.) and requires the password to use 3 of
// 4 character classes. The user-supplied secret typically does not
// satisfy these rules, so generate Azure-compliant credentials inside
// the recipe and emit them as recipe secrets - the consuming app
// receives them through CONNECTION_<NAME>_USERNAME / _PASSWORD env
// vars wired from the resource's connections.
//////////////////////////////////////////

resource "random_password" "admin" {
  length           = 24
  upper            = true
  lower            = true
  numeric          = true
  special          = true
  override_special = "!@#$%^&*()-_=+"
  min_upper        = 2
  min_lower        = 2
  min_numeric      = 2
  min_special      = 2
}

locals {
  admin_username = "mysqladmin"
  admin_password = random_password.admin.result
}

//////////////////////////////////////////
// Azure MySQL Flexible Server
//////////////////////////////////////////

resource "azurerm_mysql_flexible_server" "mysql" {
  name                = local.sanitized_server_name
  resource_group_name = data.azurerm_resource_group.rg.name
  location            = data.azurerm_resource_group.rg.location

  administrator_login    = local.admin_username
  administrator_password = local.admin_password

  sku_name = var.skuName
  version  = local.version

  backup_retention_days = 7

  storage {
    size_gb = var.storageSizeGb
  }

  tags = local.tags
}

//////////////////////////////////////////
// Firewall rule - allow Azure services
//////////////////////////////////////////

resource "azurerm_mysql_flexible_server_firewall_rule" "allow_azure" {
  name                = "AllowAzureServices"
  resource_group_name = data.azurerm_resource_group.rg.name
  server_name         = azurerm_mysql_flexible_server.mysql.name
  start_ip_address    = "0.0.0.0"
  end_ip_address      = "0.0.0.0"
}

//////////////////////////////////////////
// Database
//////////////////////////////////////////

resource "azurerm_mysql_flexible_database" "db" {
  name                = local.sanitized_database
  resource_group_name = data.azurerm_resource_group.rg.name
  server_name         = azurerm_mysql_flexible_server.mysql.name
  charset             = "utf8mb4"
  collation           = "utf8mb4_unicode_ci"
}

//////////////////////////////////////////
// Output
//////////////////////////////////////////

output "result" {
  sensitive = true
  value = {
    resources = []
    values = {
      host     = azurerm_mysql_flexible_server.mysql.fqdn
      port     = local.port
      database = local.sanitized_database
    }
    secrets = {
      username = local.admin_username
      password = local.admin_password
    }
  }
}
