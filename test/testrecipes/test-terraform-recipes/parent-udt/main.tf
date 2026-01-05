terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.37.1"
    }
  }
}

variable "context" {
  description = "This variable contains Radius recipe context."
  type = any
}

variable "port" {
  description = "Specifies the port the container listens on."
  type = number
}

locals {
  uniqueName = "usertypealpha-${substr(sha256(var.context.resource.id), 0, 8)}"
  namespace = var.context.runtime.kubernetes.namespace
}

resource "kubernetes_deployment" "usertypealpha" {
  metadata {
    name      = local.uniqueName
    namespace = local.namespace
  }

  spec {
    selector {
      match_labels = {
        app      = "usertypealpha"
        resource = var.context.resource.name
      }
    }

    template {
      metadata {
        labels = {
          app      = "usertypealpha"
          resource = var.context.resource.name
        }
      }

      spec {
        container {
          name  = "usertypealpha"
          image = "alpine:latest"
          
          port {
            container_port = var.port
          }
          
          command = ["/bin/sh"]
          args    = ["-c", "while true; do sleep 30; done"]
          
          env {
            name  = "CONN_INJECTED"
            value = try(var.context.resource.connections.externalresource.properties.configMap, "")
          }
        }
      }
    }
  }
}

output "result" {
  value = {
    # This workaround is needed because the deployment engine omits Kubernetes resources from its output.
    # Once this gap is addressed, users won't need to do this.
    resources = [
      "/planes/kubernetes/local/namespaces/${kubernetes_deployment.usertypealpha.metadata[0].namespace}/providers/apps/Deployment/${kubernetes_deployment.usertypealpha.metadata[0].name}"
    ]
    values = {
      port = var.port
    }
  }
}