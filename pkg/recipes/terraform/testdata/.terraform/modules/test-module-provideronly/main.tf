terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
      version = ">=3.0"
      configuration_aliases = [ aws.eu-west-1, aws.eu-west-2 ]
    }
  }
}