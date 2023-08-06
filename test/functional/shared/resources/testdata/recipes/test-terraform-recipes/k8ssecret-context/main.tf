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
    "resource.id" = base64encode(var.context.resource.id)
    "resource.type" = base64encode(var.context.resource.type)
    "recipe_context" = base64encode(jsonencode(var.context))
  }

  type = "Opaque"
}
