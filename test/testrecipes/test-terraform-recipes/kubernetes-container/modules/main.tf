terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.37.1"
    }
  }
}

# This is a minimal, test-only Terraform recipe for the Radius.Compute/containers
# resource type. It renders a Kubernetes Deployment (and a Service when the
# container declares ports) into the environment namespace, labeled so the Radius
# functional-test validation can find the pod. It is intentionally small: it
# exists to exercise the Terraform recipe engine and (for multi-cluster) the
# target-cluster routing for the new container type, not to reproduce the full
# production container recipe in resource-types-contrib.

locals {
  resource_name = var.context.resource.name
  namespace     = var.context.runtime.kubernetes.namespace
  app_name      = var.context.application != null ? var.context.application.name : ""

  containers = try(var.context.resource.properties.containers, {})

  labels = {
    "radapp.io/resource"    = local.resource_name
    "radapp.io/application" = local.app_name
  }

  # Normalize each container's ports into a simple list.
  container_specs = {
    for name, config in local.containers : name => {
      image = config.image
      ports = [
        for port_name, port_config in try(config.ports, {}) : {
          container_port = port_config.containerPort
        }
      ]
    }
  }

  # Flatten container ports into Service ports.
  service_ports = flatten([
    for name, spec in local.container_specs : [
      for p in spec.ports : {
        port        = p.container_port
        target_port = p.container_port
      }
    ]
  ])
}

resource "kubernetes_deployment" "container" {
  wait_for_rollout = false

  metadata {
    name      = local.resource_name
    namespace = local.namespace
    labels    = local.labels
  }

  spec {
    replicas = 1

    selector {
      match_labels = {
        "radapp.io/resource" = local.resource_name
      }
    }

    template {
      metadata {
        labels = local.labels
      }

      spec {
        dynamic "container" {
          for_each = local.container_specs
          content {
            name  = container.key
            image = container.value.image

            dynamic "port" {
              for_each = container.value.ports
              content {
                container_port = port.value.container_port
              }
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "container" {
  count = length(local.service_ports) > 0 ? 1 : 0

  metadata {
    name      = local.resource_name
    namespace = local.namespace
    labels    = local.labels
  }

  spec {
    selector = {
      "radapp.io/resource" = local.resource_name
    }

    dynamic "port" {
      for_each = local.service_ports
      content {
        port        = port.value.port
        target_port = port.value.target_port
      }
    }
  }
}

output "result" {
  value = {
    resources = concat(
      ["/planes/kubernetes/local/namespaces/${local.namespace}/providers/apps/Deployment/${local.resource_name}"],
      length(local.service_ports) > 0 ? ["/planes/kubernetes/local/namespaces/${local.namespace}/providers/core/Service/${local.resource_name}"] : []
    )
  }
}
