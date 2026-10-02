terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.100.0"
    }
  }
}

variable "name" {
  type = string
}

variable "revision" {
  type = string
}

resource "aws_s3_bucket" "managed" {
  bucket = var.name
  tags = {
    radiustest = "terraform-cloud-backend"
    revision   = var.revision
  }
}
