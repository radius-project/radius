terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
      version = "~> 3.114.0"
      configuration_aliases = [azurerm.azure-test]
    }
  }
}

resource "azurerm_resource_group" "test_rg" {
  provider = azurerm.azure-test
  name     = var.name
  location = var.location
}