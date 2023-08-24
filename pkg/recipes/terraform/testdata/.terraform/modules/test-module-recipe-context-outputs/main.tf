terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
      version = ">=3.0"
    }
  }
}

module "redis" {
  source  = "test/module/azure"

  redis_cache_name    = var.context.resource.name + var.context.aws.region
}