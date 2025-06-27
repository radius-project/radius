terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0"
    }
  }
}

variable "context" {
  description = "This variable contains Radius recipe context."
  type        = any
}

resource "kubernetes_namespace" "postgres" {
  metadata {
    # name = "postgres-${lower(element(split("/", var.context.resource.id), length(split("/", var.context.resource.id)) - 1))}"
    name = "postgres-recipe"
  }
}

# Removed PVC - using emptyDir volume instead to avoid permission issues

resource "kubernetes_deployment" "postgres" {
  metadata {
    name      = "postgres"
    namespace = kubernetes_namespace.postgres.metadata[0].name
  }

  spec {
    replicas = 1

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
          image             = "ghcr.io/ytimocin/postgres:15-alpine"
          name              = "postgres"
          image_pull_policy = "IfNotPresent"

          env {
            name  = "POSTGRES_DB"
            value = "mydb"
          }
          env {
            name  = "POSTGRES_USER"
            value = "postgres"
          }
          env {
            name  = "POSTGRES_PASSWORD"
            value = "mysecretpassword"
          }

          port {
            container_port = 5432
          }

          volume_mount {
            mount_path = "/var/lib/postgresql/data"
            name       = "postgres-storage"
          }
        }

        volume {
          name = "postgres-storage"
          empty_dir {}
        }
      }
    }
  }
}

resource "kubernetes_service" "postgres" {
  metadata {
    name      = "postgres"
    namespace = kubernetes_namespace.postgres.metadata[0].name
  }
  spec {
    selector = {
      app = "postgres"
    }
    port {
      port        = 5432
      target_port = 5432
    }
    type = "ClusterIP"
  }
}

output "result" {
  value = {
    values = {
      host     = "${kubernetes_service.postgres.metadata[0].name}.${kubernetes_namespace.postgres.metadata[0].name}.svc.cluster.local"
      port     = "5432"
      database = "mydb"
      username = "postgres"
      password = "mysecretpassword"
    }
  }
}
