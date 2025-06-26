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

variable "memory" {
  description = "Memory limits for the PostgreSQL container"
  type = map(object({
    memoryRequest = string
    memoryLimit  = string
  }))
  default = {
    S = {
      memoryRequest = "512Mi"
      memoryLimit   = "1024Mi"
    },
    M = {
      memoryRequest = "1Gi"
      memoryLimit   = "2Gi"
    }
  }
}

locals {
  uniqueName = var.context.resource.name
  port     = 5432
  namespace = var.context.runtime.kubernetes.namespace
}

resource "random_password" "password" {
  length           = 16
}

resource "kubernetes_deployment" "postgresql" {
  metadata {
    name      = local.uniqueName
    namespace = local.namespace
  }

  spec {
    selector {
      match_labels = {
        app = "postgres"
      }
    }

    template {
      metadata {
        labels = {
          app = "postgres"
        }
      }

      spec {
        container {
          image = "postgres:16-alpine"
          name  = "postgres"
          resources {
            requests = {
              memory = var.memory[var.context.resource.properties.size].memoryRequest
              }
              limits = {
                memory= var.memory[var.context.resource.properties.size].memoryLimit
              }
            }
          env {
            name  = "POSTGRES_PASSWORD"
            value = random_password.password.result
          }
          env {
            name = "POSTGRES_USER"
            value = "postgres"
          }
          env {
            name  = "POSTGRES_DB"
            value = "postgres_db"
          }
          env {
            name = "CONNECTED_TO"
            value = var.context.resource.connections.databaseresource.host
          }
          port {
            container_port = local.port
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "postgres" {
  metadata {
    name      = local.uniqueName
    namespace = local.namespace
  }

  spec {
    selector = {
      app = "postgres"
    }

    port {
      port        = local.port
      target_port = local.port
    } 
  }
}

output "result" {
  value = {
    values = {
      host = "${kubernetes_service.postgres.metadata[0].name}.${kubernetes_service.postgres.metadata[0].namespace}.svc.cluster.local"
      port = local.port
      database = "postgres_db"
      username = "postgres"
      password = random_password.password.result
    }
  }
}