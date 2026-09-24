terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.114.0"
    }
  }
}

variable "name" {
  type = string
}

variable "location" {
  type = string
}

variable "revision" {
  type = string
}

resource "azurerm_resource_group" "managed" {
  name     = var.name
  location = var.location
  tags = {
    radiustest = "terraform-cloud-backend"
    revision   = var.revision
  }
}
