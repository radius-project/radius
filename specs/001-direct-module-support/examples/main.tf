terraform {
  required_version = ">= 1.5"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.37.1"
    }
  }
}

//////////////////////////////////////////
// Direct module variables (no var.context)
//////////////////////////////////////////

variable "resource_name" {
  description = "Name of the Radius resource"
  type        = string
}

variable "application_name" {
  description = "Name of the Radius application"
  type        = string
  default     = ""
}

variable "environment_name" {
  description = "Name of the Radius environment"
  type        = string
  default     = ""
}

variable "namespace" {
  description = "Kubernetes namespace for secret lookup"
  type        = string
}

variable "database" {
  description = "MySQL database name"
  type        = string
  default     = "mysql_db"
}

variable "secret_name" {
  description = "Name of the Kubernetes secret containing DB credentials"
  type        = string
}

variable "version" {
  description = "MySQL engine version"
  type        = string
  default     = "8.4"
}

variable "vpcId" {
  description = "AWS VPC ID for the RDS instance"
  type        = string
}

variable "subnetIds" {
  description = "JSON-encoded list of subnet IDs for the DB subnet group"
  type        = string
}

variable "instanceClass" {
  description = "RDS instance class (e.g., db.t3.micro)"
  type        = string
  default     = "db.t3.micro"
}

variable "allocatedStorage" {
  description = "Allocated storage in GB"
  type        = number
  default     = 20
}

//////////////////////////////////////////
// MySQL locals
//////////////////////////////////////////

locals {
  port = 3306

  unique_suffix = substr(md5(var.resource_name), 0, 13)

  # RDS identifier: lowercase alphanumeric and hyphens, max 63 chars
  sanitized_identifier = "rds-dbinstance-${local.unique_suffix}"

  # Database name: alphanumeric and underscores only
  sanitized_database = replace(var.database, "/[^0-9A-Za-z_]/", "_")

  tags = {
    "radapp.io/resource"    = var.resource_name
    "radapp.io/application" = var.application_name
    "radapp.io/environment" = var.environment_name
  }
}

//////////////////////////////////////////
// Credentials
//////////////////////////////////////////

data "kubernetes_secret" "db_credentials" {
  metadata {
    name      = var.secret_name
    namespace = var.namespace
  }
}

//////////////////////////////////////////
// RDS security group
//////////////////////////////////////////

data "aws_vpc" "selected" {
  id = var.vpcId
}

module "rds_security_group" {
  source  = "terraform-aws-modules/security-group/aws"
  version = "~> 5.0"

  name        = "rds-sg-${local.unique_suffix}"
  description = "Security group for RDS MySQL - ${var.resource_name}"
  vpc_id      = var.vpcId

  ingress_with_cidr_blocks = [
    {
      from_port   = local.port
      to_port     = local.port
      protocol    = "tcp"
      description = "MySQL access"
      cidr_blocks = data.aws_vpc.selected.cidr_block
    }
  ]

  egress_rules = ["all-all"]

  tags = local.tags
}

//////////////////////////////////////////
// RDS instance
//////////////////////////////////////////

module "db" {
  source  = "terraform-aws-modules/rds/aws"
  version = "~> 6.0"

  identifier = local.sanitized_identifier

  engine               = "mysql"
  engine_version       = var.version
  family               = "mysql${var.version}"
  major_engine_version = var.version
  instance_class       = var.instanceClass

  db_name  = local.sanitized_database
  username = try(data.kubernetes_secret.db_credentials.data["USERNAME"], "")
  password = try(data.kubernetes_secret.db_credentials.data["PASSWORD"], "")
  port     = local.port

  allocated_storage = var.allocatedStorage
  storage_type      = "gp3"

  create_db_subnet_group = true
  db_subnet_group_name   = "rds-dbsubnetgroup-${local.unique_suffix}"
  subnet_ids             = jsondecode(var.subnetIds)

  vpc_security_group_ids = [module.rds_security_group.security_group_id]

  skip_final_snapshot = true
  apply_immediately   = true

  parameters = [
    {
      name  = "character_set_client"
      value = "utf8mb4"
    },
    {
      name  = "character_set_server"
      value = "utf8mb4"
    }
  ]

  tags = local.tags
}

//////////////////////////////////////////
// Direct outputs (no result wrapper)
//////////////////////////////////////////

output "host" {
  description = "RDS instance endpoint address"
  value       = module.db.db_instance_address
}

output "port" {
  description = "RDS instance port"
  value       = module.db.db_instance_port
}

output "database" {
  description = "Database name"
  value       = local.sanitized_database
}
