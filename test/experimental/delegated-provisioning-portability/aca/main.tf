terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "4.50.0"
    }
  }
}

variable "context" {
  type = any
}

variable "managedEnvironmentId" {
  type = string
}

locals {
  name       = "delegated-deebec8c-${substr(sha256(lower(var.context.resource.id)), 0, 10)}"
  containers = var.context.resource.properties.containers
  references = var.context.resource.incomingReferences
  port       = try(local.references[0].properties.rules[0].destinationContainer.containerPort, 80)
  valid_routes = alltrue([
    for route in local.references :
    route.type == "Radius.Compute/routes" &&
    route.properties.kind == "TCP" &&
    length(route.properties.rules) == 1 &&
    length(try(route.properties.hostnames, [])) == 0 &&
    length(route.properties.rules[0].matches) == 0 &&
    contains(keys(local.containers), route.properties.rules[0].destinationContainer.containerName) &&
    contains([
      for port in try(local.containers[route.properties.rules[0].destinationContainer.containerName].ports, {}) :
      port.containerPort
    ], route.properties.rules[0].destinationContainer.containerPort)
  ])
}

resource "azurerm_container_app" "workload" {
  name                         = local.name
  resource_group_name          = var.context.azure.resourceGroup.name
  container_app_environment_id = var.managedEnvironmentId
  revision_mode                = "Single"
  tags = {
    experiment = "delegated-provisioning-deebec8c"
  }
  template {
    min_replicas = 1
    max_replicas = 1
    dynamic "container" {
      for_each = local.containers
      content {
        name    = container.key
        image   = container.value.image
        cpu     = 0.25
        memory  = "0.5Gi"
        command = try(container.value.command, null)
        args    = try(container.value.args, null)
        dynamic "env" {
          for_each = try(container.value.env, {})
          content {
            name  = env.key
            value = env.value.value
          }
        }
      }
    }
  }
  dynamic "ingress" {
    for_each = length(local.references) == 0 ? [] : [1]
    content {
      external_enabled = false
      transport        = "tcp"
      target_port      = local.port
      exposed_port     = local.port
      traffic_weight {
        latest_revision = true
        percentage      = 100
      }
    }
  }
  lifecycle {
    precondition {
      condition     = length(local.references) <= 1 && local.valid_routes && local.port >= 1 && local.port <= 65535
      error_message = "ACA demo supports one unconditional internal TCP route to a declared container port."
    }
  }
}

output "result" {
  value = {
    resources = [azurerm_container_app.workload.id]
    values    = {}
    delegatedOutputs = [
      for route in local.references : {
        id       = route.id
        revision = route.revision
        values = {
          listener = {
            hostname = local.name
            port     = route.properties.rules[0].destinationContainer.containerPort
            protocol = "TCP"
          }
        }
      }
    ]
  }
}
