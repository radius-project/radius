terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0.3"
    }
  }
}

resource "kubernetes_secret" "recipe-context" {
  metadata {
    name = var.context.resource.name
    namespace = var.context.runtime.kubernetes.namespace
    labels = {
      "radius.dev/application" = var.context.application.name
      "radius.dev/resource" =  var.context.resource.name
    }
  }

  data = {
    "resource.id" = var.context.resource.id
    "resource.type" = var.context.resource.type
    "recipe_context" = jsonencode(var.context)
  }

  type = "Opaque"
}
