terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
      version = "~> 3.114.0"
    }
  }
}

resource "azurerm_resource_group" "test_rg" {
  name     = var.name
  location = var.location
}