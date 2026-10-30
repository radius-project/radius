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
  ports = [
    for route in local.references :
    route.properties.rules[0].destinationContainer.containerPort
  ]
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
        dynamic "container" {
          for_each = local.containers
          content {
            name    = container.key
            image   = container.value.image
            command = ["/bin/sh", "-ec"]
            args = [
              "printf 'server { listen 80; listen 8080; listen 9090; location / { return 200 \"delegated provisioning demo\\\\n\"; } }\\n' > /etc/nginx/conf.d/default.conf; exec nginx -g 'daemon off;'"
            ]
            dynamic "port" {
              for_each = container.value.ports
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
  lifecycle {
    precondition {
      condition = (
        local.valid_routes &&
        length(local.ports) == length(distinct(local.ports)) &&
        alltrue([for port in local.ports : port >= 1 && port <= 65535])
      )
      error_message = "Demo recipe requires unique TCP ports, one unconditional rule per route, and a declared destination container/port."
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
