terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
      version = "=3.7.0"
    }
  }
}

resource "random_id" "unique_name" {
  byte_length = 8
}

resource "azurerm_storage_account" "test_storage_account" {
  name = "acct${random_id.unique_name.hex}"
  resource_group_name = var.resource_group_name
  location = var.location
  account_tier = "Standard"
  account_replication_type = "LRS"
}

resource "azurerm_storage_container" "test_container" {
  name = "ctr${random_id.unique_name.hex}"
  storage_account_name = azurerm_storage_account.test_storage_account.name
}

resource "azurerm_storage_blob" "test_blob" {
  name = "blob${random_id.unique_name.hex}"
  storage_account_name = azurerm_storage_account.test_storage_account.name
  storage_container_name = azurerm_storage_container.test_container.name
  type = "Block"
}