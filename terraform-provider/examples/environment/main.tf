terraform {
  required_providers {
    radius = {
      source = "hashicorp.com/microsoft/radius"
      # version = "dev"
    }
  }
}

provider "radius" {
  api_endpoint = "test"
  api_token    = "test"
}

resource "radius_environment" "test" {
  name      = "env-from-terraform-command"
  simulated = false

  compute = {
    kind        = "kubernetes"
    resource_id = "self"
    namespace   = "default-env-from-terraform-command"

    # identity = {
    #   kind       = "azure.com.workload"
    #   oidc_issuer = "https://issuer.example.com"
    #   resource   = "resource-id"
    # }
  }

  providers = {
    azure = {
      scope = "scope-value"
    }
    aws = {
      scope = "scope-value"
    }
  }

  recipes = {
    example_recipe = {
      template_path    = "path/to/template"
      template_kind    = "bicep"
      template_version = "1.0.0"
      parameters = {
        param1 = "value1"
        param2 = "value2"
      }
    }
  }
}