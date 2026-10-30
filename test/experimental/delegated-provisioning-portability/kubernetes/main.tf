terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "2.38.0"
    }
  }
}

variable "context" {
  type = any
}

locals {
  name       = "delegated-${var.context.resource.name}"
  namespace  = var.context.runtime.kubernetes.namespace
  containers = var.context.resource.properties.containers
  references = var.context.resource.incomingReferences
}

resource "kubernetes_deployment" "workload" {
  metadata {
    name      = local.name
    namespace = local.namespace
  }
  spec {
    replicas = 1
    selector {
      match_labels = { app = local.name }
    }
    template {
      metadata {
        labels = { app = local.name }
      }
      spec {
        automount_service_account_token = false
        dynamic "container" {
          for_each = local.containers
          content {
            name    = container.key
            image   = container.value.image
            command = try(container.value.command, null)
            args    = try(container.value.args, null)
            dynamic "env" {
              for_each = try(container.value.env, {})
              content {
                name  = env.key
                value = env.value.value
              }
            }
            dynamic "port" {
              for_each = try(container.value.ports, {})
              content {
                name           = port.key
                container_port = port.value.containerPort
              }
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "ingress" {
  count = length(local.references) == 0 ? 0 : 1
  metadata {
    name      = local.name
    namespace = local.namespace
  }
  spec {
    selector = { app = local.name }
    dynamic "port" {
      for_each = {
        for route in local.references :
        basename(route.id) => route.properties.rules[0].destinationContainer
      }
      content {
        name        = port.key
        protocol    = "TCP"
        port        = port.value.containerPort
        target_port = port.value.containerPort
      }
    }
  }
  depends_on = [kubernetes_deployment.workload]
}

output "result" {
  value = {
    values = {}
    delegatedOutputs = [
      for route in local.references : {
        id       = route.id
        revision = route.revision
        values = {
          listener = {
            hostname = "${local.name}.${local.namespace}.svc.cluster.local"
            port     = route.properties.rules[0].destinationContainer.containerPort
            protocol = "TCP"
          }
        }
      }
    ]
  }
}
