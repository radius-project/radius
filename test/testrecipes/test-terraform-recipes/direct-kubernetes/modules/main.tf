terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.37.1"
    }
  }
}

# This is a "direct" module: it has no `context` input variable and no structured
# `result` output. Radius resolves the `name`/`namespace` parameters from
# {{context.*}} expressions in the recipe definition and maps the plain outputs
# below onto resource properties via the recipe's `outputs` field.
resource "kubernetes_deployment" "redis" {
  metadata {
    name      = var.name
    namespace = var.namespace
    labels = {
      app = "redis"
    }
  }

  spec {
    replicas = 1

    selector {
      match_labels = {
        app = "redis"
      }
    }

    template {
      metadata {
        labels = {
          app = "redis"
        }
      }

      spec {
        container {
          name  = "redis"
          image = "ghcr.io/radius-project/mirror/redis:6.2"
          port {
            container_port = var.port
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "redis" {
  metadata {
    name      = var.name
    namespace = var.namespace
  }

  spec {
    selector = {
      app = "redis"
    }

    port {
      port        = var.port
      target_port = var.port
    }
  }
}
