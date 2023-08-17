output "result" {
  value = {
    resources = [azurerm_storage_account.test_storage_account.id]
  }
}