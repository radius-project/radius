terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0"
    }
  }
}

resource "kubernetes_secret" "recipe-context" {
  metadata {
    name = var.context.resource.name
    namespace = var.context.runtime.kubernetes.namespace

    # Add labels to enable functional test recognize the created resource.
    labels = {
      "radius.dev/application" = var.context.application.name
      "radius.dev/resource" =  var.context.resource.name
      "radius.dev/resource-type" = "applications.core-extenders"
    }
  }

  data = {
    "resource.id" = base64encode(var.context.resource.id)
    "resource.type" = base64encode(var.context.resource.type)
    "azure.subscription_id" = base64encode(var.context.azure.subscription.subscriptionId)
    # Serialize the entire recipe context object. Functional test code will decode and assert the entire values.
    "recipe_context" = base64encode(jsonencode(var.context))
  }

  type = "Opaque"
}
