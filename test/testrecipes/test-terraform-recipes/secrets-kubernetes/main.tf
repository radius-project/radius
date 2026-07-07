terraform {
  required_version = ">= 1.5"

  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0"
    }
  }
}

//////////////////////////////////////////
// Common Radius variables
//////////////////////////////////////////

locals {
  resource_name    = var.context.resource.name
  application_name = var.context.application != null ? var.context.application.name : ""
  environment_name = var.context.environment != null ? var.context.environment.name : ""

  # Under the Radius.Core/applications model the application-scoped namespace
  # (runtime.kubernetes.namespace) can be empty. Fall back to the environment
  # namespace, then to "default", so the Secret always has a namespace.
  app_namespace = try(var.context.runtime.kubernetes.namespace, "")
  env_namespace = try(var.context.runtime.kubernetes.environmentNamespace, "")
  namespace     = local.app_namespace != "" ? local.app_namespace : (local.env_namespace != "" ? local.env_namespace : "default")
}

//////////////////////////////////////////
// Secret data
//
// data is shaped as { key: { value: "...", encoding: "..." } }. Entries with
// encoding == "base64" are passed through binary_data (already base64), the
// rest go into data (the provider base64-encodes them).
//////////////////////////////////////////

locals {
  secret_data = try(var.context.resource.properties.data, {})

  string_data = {
    for k, v in local.secret_data : k => v.value
    if try(v.encoding, "") != "base64"
  }

  binary_data = {
    for k, v in local.secret_data : k => v.value
    if try(v.encoding, "") == "base64"
  }

  kind = try(var.context.resource.properties.kind, "generic")
  secret_type = local.kind == "certificate-pem" ? "kubernetes.io/tls" : (
    local.kind == "basicAuthentication" ? "kubernetes.io/basic-auth" : "Opaque"
  )
}

//////////////////////////////////////////
// Kubernetes Secret
//////////////////////////////////////////

resource "kubernetes_secret" "secret" {
  metadata {
    name      = local.resource_name
    namespace = local.namespace
    labels = {
      resource = local.resource_name
      app      = local.application_name
    }
  }

  data        = local.string_data
  binary_data = local.binary_data
  type        = local.secret_type
}

//////////////////////////////////////////
// Output
//////////////////////////////////////////

output "result" {
  value = {
    resources = [
      "/planes/kubernetes/local/namespaces/${local.namespace}/providers/core/Secret/${local.resource_name}"
    ]
  }
}
