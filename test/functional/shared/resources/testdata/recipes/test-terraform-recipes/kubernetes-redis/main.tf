terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0"
    }
  }
}

resource "kubernetes_deployment" "redis" {
  metadata {
    name = var.redis_cache_name
    namespace = var.context.runtime.kubernetes.namespace
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
          image = "redis:latest" 
          port {
            container_port = 6379
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "redis" {
  metadata {
    name = var.redis_cache_name
    namespace = var.context.runtime.kubernetes.namespace
  }

  spec {
    selector = {
      app = "redis"
    }

    port {
      port        = 6379  # Service port
      target_port = 6379  # Target port of the Redis deployment
    }
  }
}