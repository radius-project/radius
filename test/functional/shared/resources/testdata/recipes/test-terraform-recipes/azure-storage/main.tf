terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
      version = "~> 3.0.0"
    }
  }
}

resource "azurerm_storage_account" "test_storage_account" {
  name = var.name
  resource_group_name = var.resource_group_name
  location = var.location
  account_tier = "Standard"
  account_replication_type = "LRS"
}

resource "azurerm_storage_container" "test_container" {
  name = "test-container"
  storage_account_name = azurerm_storage_account.test_storage_account.name
}

resource "azurerm_storage_blob" "test_blob" {
  name = "test-blob"
  storage_account_name = azurerm_storage_account.test_storage_account.name
  storage_container_name = azurerm_storage_container.test_container.name
  type = "Block"
}